#!/usr/bin/env python3
"""Evaluate a Qwen 3.5 2B + Spokes model on mnemonic encoding and compression tasks.

Two evaluation modes:

  Mode A: Loss (default)
    Compute cross-entropy loss on completion tokens only (loss masking).

  Mode B: Generation
    Greedy decode from prompt, parse output as JSON, evaluate quality
    against schema and (optionally) ground-truth responses.

Usage:
    # Loss evaluation on spoke checkpoint
    python eval_qwen_encoding.py --base-model Qwen/Qwen3.5-2B --spokes checkpoints/spokes_best.pt

    # Generation quality (greedy decode + JSON parse + schema check)
    python eval_qwen_encoding.py --base-model Qwen/Qwen3.5-2B --spokes checkpoints/spokes_best.pt --mode generate

    # Novel input test (the test that caught Felix's failures)
    python eval_qwen_encoding.py --base-model Qwen/Qwen3.5-2B --spokes checkpoints/spokes_best.pt --mode novel

Requires:
    - transformers: pip install transformers
    - Felix-LM venv for validate.py: source ~/Projects/felixlm/.venv/bin/activate
"""

import argparse
import json
import math
import sys
from dataclasses import dataclass, field
from pathlib import Path

import torch
import torch.nn.functional as F
from transformers import AutoTokenizer

TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))

from validate import validate_encoding  # noqa: E402

# Required fields for schema compliance
FULL_REQUIRED_FIELDS = {
    "gist", "summary", "content", "narrative", "concepts",
    "structured_concepts", "significance", "emotional_tone",
    "outcome", "salience",
}

MINIMAL_REQUIRED_FIELDS = {"summary", "concepts", "salience"}

# Novel inputs for generalization testing — completely outside training distribution
NOVEL_INPUTS = [
    # Developer decisions
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "Decision: switched from REST to gRPC for inter-service communication because latency was too high at 200ms p99. The team evaluated both options over a week-long spike. gRPC brought it down to 12ms p99 but required regenerating all client stubs.",
    },
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "We decided to use SQLite WAL mode instead of rollback journal because the benchmark showed 3x write throughput improvement with concurrent readers. The downside is WAL files can grow unbounded if checkpointing fails.",
    },
    # Error reports
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "Bug: the consolidation agent crashes with a nil pointer when processing memories that have zero associations. Root cause was a missing nil check in spread_activation.go line 142. Fixed by guarding the association slice access.",
    },
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "Error: PyTorch ROCm 2.9.1 segfaults when calling torch.compile with fullgraph=True on the RX 7800 XT. Only happens with bf16 tensors larger than 2GB. Workaround: disable fullgraph mode or use float32.",
    },
    # Code/architecture discussions
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "The event bus uses an in-memory pub/sub pattern. Agents subscribe to event types and receive callbacks. The orchestrator publishes health checks every 30 seconds. There's no persistence — if the daemon restarts, all subscriptions are re-established from agent init code.",
    },
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "Refactored the embedding pipeline to batch requests. Previously each memory was embedded individually (1 API call per memory). Now we batch up to 32 memories per call, reducing total embedding time from 45 seconds to 3 seconds for a typical consolidation cycle of 200 memories.",
    },
    # Edge cases
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "ok",
    },
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "```go\nfunc (s *Store) GetMemory(id string) (*Memory, error) {\n\trow := s.db.QueryRow(\"SELECT id, content, salience FROM memories WHERE id = ?\", id)\n\tvar m Memory\n\tif err := row.Scan(&m.ID, &m.Content, &m.Salience); err != nil {\n\t\treturn nil, fmt.Errorf(\"get memory %s: %w\", id, err)\n\t}\n\treturn &m, nil\n}\n```",
    },
    {
        "system": "Compress the following text into the most compact representation possible while preserving all key facts. Output only the compressed form.",
        "user": "The quarterly review meeting was held on March 15, 2026 at the downtown office. Sarah Chen presented the Q1 results: revenue up 23% year-over-year to $4.2M, customer churn reduced from 8.1% to 5.3%, and the new enterprise tier launched with 12 initial customers. The board approved the Series B timeline for Q3.",
    },
    {
        "system": "You are a memory encoder. You receive events and output structured JSON. Never explain, never apologize.",
        "user": "Mnemonic daemon健康状態: すべてのエージェントが正常に動作しています。メモリ数は1,234件、エンコーディングキューは空です。",
    },
]


@dataclass
class EvalResults:
    """Container for evaluation metrics."""
    # Loss metrics
    total_loss: float = 0.0
    total_tokens: int = 0
    n_examples: int = 0

    # Generation metrics
    json_valid: int = 0
    json_invalid: int = 0
    schema_full_pass: int = 0
    schema_minimal_pass: int = 0
    hard_failures: list = field(default_factory=list)
    unique_gists: set = field(default_factory=set)

    # Concept metrics
    concept_jaccard_scores: list = field(default_factory=list)
    salience_errors: list = field(default_factory=list)

    @property
    def mean_loss(self) -> float:
        return self.total_loss / max(self.total_tokens, 1)

    @property
    def perplexity(self) -> float:
        return math.exp(min(self.mean_loss, 100))

    @property
    def json_validity_rate(self) -> float:
        total = self.json_valid + self.json_invalid
        return self.json_valid / max(total, 1)

    @property
    def schema_full_rate(self) -> float:
        total = self.json_valid + self.json_invalid
        return self.schema_full_pass / max(total, 1)

    @property
    def mean_concept_jaccard(self) -> float:
        if not self.concept_jaccard_scores:
            return 0.0
        return sum(self.concept_jaccard_scores) / len(self.concept_jaccard_scores)

    @property
    def mean_salience_error(self) -> float:
        if not self.salience_errors:
            return 0.0
        return sum(self.salience_errors) / len(self.salience_errors)

    def print_summary(self, mode: str = "loss"):
        print(f"\n{'='*60}")
        print(f"  EVALUATION RESULTS ({mode.upper()})")
        print(f"{'='*60}")

        if mode == "loss":
            print(f"  Examples:     {self.n_examples}")
            print(f"  Tokens:       {self.total_tokens:,}")
            print(f"  Mean loss:    {self.mean_loss:.4f}")
            print(f"  Perplexity:   {self.perplexity:.2f}")

        if mode in ("generate", "novel"):
            total = self.json_valid + self.json_invalid
            print(f"  Examples:           {total}")
            print(f"  JSON valid:         {self.json_valid}/{total} ({self.json_validity_rate*100:.1f}%)")
            print(f"  Schema (full):      {self.schema_full_pass}/{total} ({self.schema_full_rate*100:.1f}%)")
            print(f"  Schema (minimal):   {self.schema_minimal_pass}/{total}")
            print(f"  Unique gists:       {len(self.unique_gists)}/{total}")

            if self.concept_jaccard_scores:
                print(f"  Concept Jaccard:    {self.mean_concept_jaccard:.3f}")
            if self.salience_errors:
                print(f"  Salience MAE:       {self.mean_salience_error:.3f}")
            if self.hard_failures:
                from collections import Counter
                counts = Counter(self.hard_failures)
                print(f"  Hard failures:")
                for f, c in counts.most_common(5):
                    print(f"    {f}: {c}")

            # Template memorization check
            if len(self.unique_gists) < total * 0.5:
                print(f"\n  WARNING: Only {len(self.unique_gists)} unique gists out of {total} — possible template memorization!")

        print(f"{'='*60}")


def load_model(base_model_path: str, spoke_path: str | None, device: torch.device):
    """Load Qwen 3.5 2B with optional spoke weights."""
    from qwen_spoke_adapter import QwenWithSpokes, SpokeConfig

    if spoke_path:
        # Load spoke config from checkpoint
        data = torch.load(spoke_path, weights_only=True, map_location="cpu")
        spoke_config = SpokeConfig(**data["spoke_config"])
    else:
        spoke_config = SpokeConfig()

    model = QwenWithSpokes.from_pretrained(
        base_model_path,
        spoke_config=spoke_config,
        torch_dtype=torch.bfloat16,
    )

    if spoke_path:
        model.load_spokes(spoke_path)

    model.to(device)
    model.eval()
    return model


def eval_loss(model, tokenizer, eval_path: str, device: torch.device, max_examples: int = 0) -> EvalResults:
    """Evaluate cross-entropy loss on completion tokens only."""
    results = EvalResults()

    with open(eval_path) as f:
        for i, line in enumerate(f):
            if max_examples and i >= max_examples:
                break

            example = json.loads(line)
            input_ids = torch.tensor([example["input_ids"]], device=device)
            completion_start = example["completion_start"]

            with torch.no_grad():
                outputs = model(input_ids=input_ids)
                logits = outputs.logits

            # Loss only on completion tokens (after completion_start)
            shift_logits = logits[:, completion_start - 1 : -1, :].contiguous()
            shift_labels = input_ids[:, completion_start:].contiguous()

            loss = F.cross_entropy(
                shift_logits.view(-1, shift_logits.size(-1)),
                shift_labels.view(-1),
                reduction="sum",
            )

            n_tokens = shift_labels.numel()
            results.total_loss += loss.item()
            results.total_tokens += n_tokens
            results.n_examples += 1

            if (i + 1) % 50 == 0:
                print(f"  [{i+1}] running loss={results.mean_loss:.4f} PPL={results.perplexity:.2f}")

    return results


def eval_generation(
    model, tokenizer, inputs: list[dict], device: torch.device, max_new_tokens: int = 1024
) -> EvalResults:
    """Evaluate generation quality on a list of (system, user) inputs."""
    results = EvalResults()

    for i, inp in enumerate(inputs):
        system = inp["system"]
        user = inp["user"]
        ground_truth = inp.get("response")

        # Build prompt using chat template
        messages = [{"role": "system", "content": system}, {"role": "user", "content": user}]
        prompt = tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)

        # Inject </think> to skip reasoning
        prompt += "</think>\n\n"

        input_ids = tokenizer.encode(prompt, return_tensors="pt").to(device)

        with torch.no_grad():
            output_ids = model.base_model.generate(
                input_ids,
                max_new_tokens=max_new_tokens,
                do_sample=False,
                temperature=None,
                top_p=None,
            )

        # Decode only the generated part
        generated_ids = output_ids[0, input_ids.shape[1]:]
        generated_text = tokenizer.decode(generated_ids, skip_special_tokens=True).strip()

        print(f"\n  [{i+1}/{len(inputs)}] Input: {user[:80]}...")
        print(f"  Output: {generated_text[:200]}...")

        # Check JSON validity
        try:
            parsed = json.loads(generated_text)
            results.json_valid += 1

            # Schema compliance
            if isinstance(parsed, dict):
                keys = set(parsed.keys())
                if FULL_REQUIRED_FIELDS.issubset(keys):
                    results.schema_full_pass += 1
                if MINIMAL_REQUIRED_FIELDS.issubset(keys):
                    results.schema_minimal_pass += 1

                # Track unique gists (template memorization check)
                gist = parsed.get("gist", "")
                if gist:
                    results.unique_gists.add(gist)

                # Concept Jaccard (if ground truth available)
                if ground_truth:
                    try:
                        gt_parsed = json.loads(ground_truth)
                        gt_concepts = set(gt_parsed.get("concepts", []))
                        pred_concepts = set(parsed.get("concepts", []))
                        if gt_concepts or pred_concepts:
                            jaccard = len(gt_concepts & pred_concepts) / len(gt_concepts | pred_concepts)
                            results.concept_jaccard_scores.append(jaccard)
                    except (json.JSONDecodeError, TypeError):
                        pass

                # Salience error
                salience = parsed.get("salience")
                if ground_truth and salience is not None:
                    try:
                        gt_parsed = json.loads(ground_truth)
                        gt_salience = gt_parsed.get("salience")
                        if gt_salience is not None:
                            results.salience_errors.append(abs(salience - gt_salience))
                    except (json.JSONDecodeError, TypeError):
                        pass

            # Run validate.py quality gates
            vr = validate_encoding(generated_text)
            if not vr.valid:
                results.hard_failures.extend(vr.hard_failures)

        except (json.JSONDecodeError, TypeError):
            results.json_invalid += 1
            # Check for degenerate repetition (EXP-10 failure mode)
            if len(generated_text) > 100:
                # Check if any 20-char substring repeats 3+ times
                for j in range(len(generated_text) - 60):
                    chunk = generated_text[j : j + 20]
                    if generated_text.count(chunk) >= 3:
                        results.hard_failures.append("degenerate_repetition")
                        print(f"  WARNING: Degenerate repetition detected!")
                        break
            print(f"  FAIL: Not valid JSON")

    return results


def eval_novel(model, tokenizer, device: torch.device) -> EvalResults:
    """Run the novel input test — the test that caught Felix-LM's failures."""
    print("\n--- Novel Input Test ---")
    print(f"Testing {len(NOVEL_INPUTS)} inputs outside training distribution\n")
    return eval_generation(model, tokenizer, NOVEL_INPUTS, device)


def main():
    parser = argparse.ArgumentParser(description="Evaluate Qwen 3.5 2B + Spokes")
    parser.add_argument("--base-model", default="Qwen/Qwen3.5-2B", help="Base model path or HF name")
    parser.add_argument("--spokes", default=None, help="Path to spoke weights checkpoint")
    parser.add_argument("--eval-data", default=None, help="Path to eval JSONL (for loss mode)")
    parser.add_argument("--mode", choices=["loss", "generate", "novel", "all"], default="loss")
    parser.add_argument("--max-examples", type=int, default=0, help="Max examples for loss eval (0=all)")
    parser.add_argument("--max-new-tokens", type=int, default=1024, help="Max tokens to generate")
    parser.add_argument("--device", default="auto", help="Device (auto, cpu, cuda, cuda:0)")
    parser.add_argument("--tokenizer-path", default=None, help="Path to tokenizer")
    args = parser.parse_args()

    # Device selection
    if args.device == "auto":
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    else:
        device = torch.device(args.device)
    print(f"Device: {device}")

    # Load tokenizer
    tok_path = args.tokenizer_path or args.base_model
    tokenizer = AutoTokenizer.from_pretrained(tok_path)

    # Load model
    model = load_model(args.base_model, args.spokes, device)

    # Run evaluations
    if args.mode in ("loss", "all"):
        eval_path = args.eval_data
        if not eval_path:
            eval_path = str(TRAINING_DIR / "data" / "finetune_qwen" / "eval.jsonl")
        if Path(eval_path).exists():
            print(f"\n--- Loss Evaluation: {eval_path} ---")
            results = eval_loss(model, tokenizer, eval_path, device, args.max_examples)
            results.print_summary("loss")
        else:
            print(f"Eval data not found: {eval_path}")

    if args.mode in ("generate", "all"):
        eval_path = args.eval_data
        if not eval_path:
            eval_path = str(TRAINING_DIR / "data" / "finetune_qwen" / "eval.jsonl")
        if Path(eval_path).exists():
            # Load eval examples and convert to generation format
            inputs = []
            with open(eval_path) as f:
                for line in f:
                    example = json.loads(line)
                    ids = example["input_ids"]
                    cs = example["completion_start"]
                    prefix_text = tokenizer.decode(ids[:cs])
                    completion_text = tokenizer.decode(ids[cs:])

                    # Extract system/user from decoded prefix
                    # This is approximate — good enough for eval
                    inputs.append({
                        "system": "You are a memory encoder. You receive events and output structured JSON.",
                        "user": prefix_text.split("<|im_start|>user\n")[-1].split("<|im_end|>")[0] if "<|im_start|>user" in prefix_text else prefix_text[-500:],
                        "response": completion_text,
                    })
                    if len(inputs) >= 50:
                        break

            print(f"\n--- Generation Evaluation: {len(inputs)} examples ---")
            results = eval_generation(model, tokenizer, inputs, device)
            results.print_summary("generate")

    if args.mode in ("novel", "all"):
        results = eval_novel(model, tokenizer, device)
        results.print_summary("novel")

    # Clean up hooks
    model.remove_hooks()


if __name__ == "__main__":
    main()
