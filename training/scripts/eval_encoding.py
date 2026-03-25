#!/usr/bin/env python3
"""Evaluate a fine-tuned Felix-LM v3 model on the mnemonic encoding task.

Two evaluation modes:

  Mode A: Perplexity (default)
    Compute cross-entropy loss on completion tokens only.

  Mode B: Generation
    Greedy decode from prompt, parse output as JSON, evaluate quality
    against the schema and (optionally) ground-truth Gemini responses.

Usage:
    # Perplexity on finetuned checkpoint
    python training/scripts/eval_encoding.py --checkpoint checkpoints/finetune/last.pt

    # Generation quality
    python training/scripts/eval_encoding.py --checkpoint checkpoints/finetune/last.pt --mode generate

    # Compare pretrained (unfinetuned) baseline
    python training/scripts/eval_encoding.py --checkpoint checkpoints/v3_mnemonic_100m/last.pt

Requires:
    - Felix-LM installed/importable: pip install -e ~/Projects/felixlm
    - tokenizers package: pip install tokenizers
    - Eval JSONL from prepare_finetune_data.py (has input_ids, completion_start, seq_len)
"""

import argparse
import json
import math
import sys
from pathlib import Path

import torch
import torch.nn.functional as F

# ---------------------------------------------------------------------------
# Reuse quality gates from validate.py
# ---------------------------------------------------------------------------
TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))

from validate import validate_encoding  # noqa: E402

# Required fields for schema compliance (minimal set that every encoding must have)
REQUIRED_FIELDS = {"summary", "concepts", "salience"}

# Full required fields matching the encoding schema
FULL_REQUIRED_FIELDS = {
    "gist", "summary", "content", "narrative", "concepts",
    "structured_concepts", "significance", "emotional_tone",
    "outcome", "salience",
}

# ---------------------------------------------------------------------------
# Model loading (matches train_mnemonic_lm.py patterns)
# ---------------------------------------------------------------------------

def make_v3_mnemonic_100m_config():
    """Felix-LM v3 at ~100M params with mnemonic's 32K vocab."""
    from felix_lm.v3.config import FelixV3Config

    return FelixV3Config(
        vocab_size=32768,
        d_embed=512,
        num_layers=20,
        num_heads=8,
        num_spokes=4,
        spoke_rank=64,
        gate_schedule="uniform",
        embed_proj=True,
        dropout=0.0,  # No dropout at eval time
        gradient_checkpointing=False,
    )


def load_model(checkpoint_path: str, device: torch.device) -> torch.nn.Module:
    """Load a Felix-LM v3 100M model from checkpoint.

    Handles both checkpoint formats:
      - Full checkpoint dict with 'model_state_dict' key
      - Legacy raw state_dict
    Also strips _orig_mod. prefix from torch.compile'd checkpoints.
    """
    from felix_lm.v3.model import FelixLMv3

    config = make_v3_mnemonic_100m_config()
    model = FelixLMv3(config)

    ckpt = torch.load(checkpoint_path, map_location=device, weights_only=False)

    if isinstance(ckpt, dict) and "model_state_dict" in ckpt:
        state_dict = ckpt["model_state_dict"]
    else:
        state_dict = ckpt

    # Strip _orig_mod. prefix if saved from a torch.compile'd model
    if any(k.startswith("_orig_mod.") for k in state_dict.keys()):
        state_dict = {
            k.replace("_orig_mod.", "", 1): v for k, v in state_dict.items()
        }

    model.load_state_dict(state_dict)
    model.to(device)
    model.eval()
    return model


# ---------------------------------------------------------------------------
# Eval data loading
# ---------------------------------------------------------------------------

def load_eval_data(eval_path: str) -> list[dict]:
    """Load eval JSONL produced by prepare_finetune_data.py.

    Each line is a JSON object with keys:
      - input_ids: list[int]  (full sequence: prompt + completion)
      - completion_start: int (index where completion tokens begin)
      - seq_len: int
    May also contain ground-truth fields for comparison.
    """
    examples = []
    with open(eval_path) as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                ex = json.loads(line)
            except json.JSONDecodeError:
                print(f"Warning: skipping malformed line {line_num} in {eval_path}")
                continue

            if "input_ids" not in ex or "completion_start" not in ex:
                print(f"Warning: skipping line {line_num} — missing input_ids or completion_start")
                continue

            examples.append(ex)

    return examples


# ---------------------------------------------------------------------------
# Mode A: Perplexity evaluation
# ---------------------------------------------------------------------------

def eval_perplexity(
    model: torch.nn.Module,
    examples: list[dict],
    device: torch.device,
    tokenizer_path: str | None = None,
    verbose: bool = False,
) -> dict:
    """Compute cross-entropy loss, PPL, and BPB on completion tokens only."""
    total_loss = 0.0
    total_tokens = 0
    total_bytes = 0
    per_example = []

    # Load tokenizer for BPB calculation (decode tokens -> count bytes)
    tok = None
    if tokenizer_path:
        from tokenizers import Tokenizer
        tok = Tokenizer.from_file(tokenizer_path)

    for i, ex in enumerate(examples):
        input_ids = torch.tensor([ex["input_ids"]], dtype=torch.long, device=device)
        completion_start = ex["completion_start"]
        seq_len = len(ex["input_ids"])

        if completion_start >= seq_len - 1:
            if verbose:
                print(f"  Example {i}: skipped (no completion tokens)")
            continue

        with torch.no_grad():
            result = model(input_ids)
            logits = result["logits"]  # [1, seq_len, vocab_size]

        # Shift: predict token t+1 from position t
        # We only care about positions [completion_start-1, seq_len-2] predicting
        # tokens [completion_start, seq_len-1]
        shift_logits = logits[0, completion_start - 1 : seq_len - 1, :]  # [num_completion, vocab]
        shift_targets = input_ids[0, completion_start : seq_len]  # [num_completion]

        loss = F.cross_entropy(shift_logits, shift_targets, reduction="sum")
        num_tokens = shift_targets.numel()

        # Count bytes in completion text for BPB
        num_bytes = 0
        if tok is not None:
            completion_token_ids = ex["input_ids"][completion_start:seq_len]
            completion_text = tok.decode(completion_token_ids)
            num_bytes = len(completion_text.encode("utf-8"))

        example_loss = loss.item() / num_tokens
        example_ppl = math.exp(min(example_loss, 20.0))
        example_bpb = (loss.item() / math.log(2)) / num_bytes if num_bytes > 0 else float("nan")

        per_example.append({
            "index": i,
            "loss": example_loss,
            "ppl": example_ppl,
            "bpb": example_bpb,
            "num_completion_tokens": num_tokens,
            "num_completion_bytes": num_bytes,
        })

        total_loss += loss.item()
        total_tokens += num_tokens
        total_bytes += num_bytes

        if verbose:
            print(f"  Example {i}: loss={example_loss:.4f}  ppl={example_ppl:.2f}  bpb={example_bpb:.3f}  tokens={num_tokens}  bytes={num_bytes}")

    if total_tokens == 0:
        print("Error: no completion tokens found in eval data.")
        return {"mean_loss": float("nan"), "ppl": float("nan"), "bpb": float("nan"), "total_tokens": 0, "per_example": []}

    mean_loss = total_loss / total_tokens
    ppl = math.exp(min(mean_loss, 20.0))
    bpb = (total_loss / math.log(2)) / total_bytes if total_bytes > 0 else float("nan")

    # Loss distribution
    losses = [e["loss"] for e in per_example]
    losses_sorted = sorted(losses)
    n = len(losses_sorted)

    return {
        "mean_loss": mean_loss,
        "ppl": ppl,
        "bpb": bpb,
        "total_tokens": total_tokens,
        "total_bytes": total_bytes,
        "num_examples": len(per_example),
        "loss_min": losses_sorted[0] if n else float("nan"),
        "loss_p25": losses_sorted[n // 4] if n else float("nan"),
        "loss_median": losses_sorted[n // 2] if n else float("nan"),
        "loss_p75": losses_sorted[3 * n // 4] if n else float("nan"),
        "loss_max": losses_sorted[-1] if n else float("nan"),
        "per_example": per_example,
    }


# ---------------------------------------------------------------------------
# Mode B: Generation evaluation
# ---------------------------------------------------------------------------

def greedy_decode(
    model: torch.nn.Module,
    prompt_ids: list[int],
    max_tokens: int,
    device: torch.device,
    temperature: float = 0.0,
    eos_token: int = 0,
    max_context: int = 4096,
) -> list[int]:
    """Greedy (or temperature-sampled) autoregressive decode.

    Uses a sliding context window to avoid reprocessing the full
    prompt+generated sequence on every step. Only the last
    `max_context` tokens are passed to the model.
    """
    all_ids = list(prompt_ids)
    generated = []

    for _ in range(max_tokens):
        # Sliding window: only feed last max_context tokens
        context = all_ids[-max_context:]
        input_ids = torch.tensor([context], dtype=torch.long, device=device)

        with torch.no_grad():
            result = model(input_ids)
            logits = result["logits"]  # [1, seq_len, vocab_size]

        next_logits = logits[0, -1, :]  # [vocab_size]

        if temperature <= 0.0:
            next_token = next_logits.argmax().item()
        else:
            probs = F.softmax(next_logits / temperature, dim=-1)
            next_token = torch.multinomial(probs, 1).item()

        if next_token == eos_token:
            break

        generated.append(next_token)
        all_ids.append(next_token)

    return generated


def jaccard_similarity(set_a: set, set_b: set) -> float:
    """Jaccard similarity between two sets."""
    if not set_a and not set_b:
        return 1.0
    if not set_a or not set_b:
        return 0.0
    return len(set_a & set_b) / len(set_a | set_b)


def evaluate_generation(
    generated_text: str,
    ground_truth_text: str | None = None,
) -> dict:
    """Evaluate a single generated encoding against schema and ground truth."""
    result = {
        "json_valid": False,
        "schema_compliant": False,
        "field_quality": {},
        "ground_truth_comparison": {},
    }

    # JSON validity
    try:
        data = json.loads(generated_text)
        result["json_valid"] = True
    except (json.JSONDecodeError, TypeError):
        return result

    if not isinstance(data, dict):
        return result

    # Schema compliance: check minimal required fields
    has_required = all(f in data for f in REQUIRED_FIELDS)
    result["schema_compliant"] = has_required

    # Check full required fields
    has_full = all(f in data for f in FULL_REQUIRED_FIELDS)
    result["full_schema_compliant"] = has_full

    # Field quality checks
    fq = {}

    # summary length
    summary = data.get("summary", "")
    if isinstance(summary, str):
        fq["summary_len"] = len(summary)
        fq["summary_ok"] = len(summary) <= 100
    else:
        fq["summary_ok"] = False

    # gist length
    gist = data.get("gist", "")
    if isinstance(gist, str):
        fq["gist_len"] = len(gist)
        fq["gist_ok"] = len(gist) <= 60
    else:
        fq["gist_ok"] = False

    # salience range
    salience = data.get("salience")
    if isinstance(salience, (int, float)):
        fq["salience"] = salience
        fq["salience_ok"] = 0.0 <= salience <= 1.0
    else:
        fq["salience_ok"] = False

    # concepts is a list of strings
    concepts = data.get("concepts")
    if isinstance(concepts, list) and all(isinstance(c, str) for c in concepts):
        fq["num_concepts"] = len(concepts)
        fq["concepts_ok"] = True
    else:
        fq["concepts_ok"] = False

    # Run full validation gates
    validation = validate_encoding(generated_text)
    fq["hard_failures"] = validation.hard_failures
    fq["soft_warnings"] = validation.soft_warnings
    fq["passes_hard_gates"] = validation.valid

    result["field_quality"] = fq

    # Ground truth comparison
    if ground_truth_text:
        try:
            gt = json.loads(ground_truth_text)
            if isinstance(gt, dict):
                gt_compare = {}

                # Concept overlap (Jaccard)
                gen_concepts = set(
                    c.lower() for c in data.get("concepts", []) if isinstance(c, str)
                )
                gt_concepts = set(
                    c.lower() for c in gt.get("concepts", []) if isinstance(c, str)
                )
                gt_compare["concept_jaccard"] = jaccard_similarity(gen_concepts, gt_concepts)
                gt_compare["gen_concepts"] = sorted(gen_concepts)
                gt_compare["gt_concepts"] = sorted(gt_concepts)

                # Salience delta
                gen_sal = data.get("salience")
                gt_sal = gt.get("salience")
                if isinstance(gen_sal, (int, float)) and isinstance(gt_sal, (int, float)):
                    gt_compare["salience_delta"] = abs(gen_sal - gt_sal)
                    gt_compare["gen_salience"] = gen_sal
                    gt_compare["gt_salience"] = gt_sal

                result["ground_truth_comparison"] = gt_compare
        except (json.JSONDecodeError, TypeError):
            pass

    return result


def eval_generate(
    model: torch.nn.Module,
    examples: list[dict],
    device: torch.device,
    tokenizer,
    max_tokens: int = 512,
    temperature: float = 0.0,
    verbose: bool = False,
) -> dict:
    """Generate encodings and evaluate quality."""
    from tqdm import tqdm

    results = []

    for i, ex in enumerate(tqdm(examples, desc="Generating", unit="ex")):
        input_ids = ex["input_ids"]
        completion_start = ex["completion_start"]

        # Feed only prompt tokens
        prompt_ids = input_ids[:completion_start]

        # Ground truth: decode the completion portion
        gt_token_ids = input_ids[completion_start:]
        # Strip trailing EOS/pad (token 0)
        while gt_token_ids and gt_token_ids[-1] == 0:
            gt_token_ids = gt_token_ids[:-1]
        ground_truth_text = tokenizer.decode(gt_token_ids) if gt_token_ids else None

        # Generate
        gen_ids = greedy_decode(
            model, prompt_ids, max_tokens, device,
            temperature=temperature, eos_token=0,
        )
        generated_text = tokenizer.decode(gen_ids) if gen_ids else ""

        # Evaluate
        eval_result = evaluate_generation(generated_text, ground_truth_text)
        eval_result["index"] = i
        eval_result["num_generated_tokens"] = len(gen_ids)
        eval_result["generated_text"] = generated_text

        results.append(eval_result)

        if verbose:
            status = "OK" if eval_result["json_valid"] else "INVALID JSON"
            schema = "schema OK" if eval_result["schema_compliant"] else "schema FAIL"
            print(f"\n--- Example {i} [{status}, {schema}] ---")
            print(f"  Generated tokens: {len(gen_ids)}")
            if eval_result["json_valid"]:
                fq = eval_result["field_quality"]
                print(f"  Summary len: {fq.get('summary_len', '?')}, OK: {fq.get('summary_ok', '?')}")
                print(f"  Gist len: {fq.get('gist_len', '?')}, OK: {fq.get('gist_ok', '?')}")
                print(f"  Salience: {fq.get('salience', '?')}, OK: {fq.get('salience_ok', '?')}")
                print(f"  Concepts: {fq.get('num_concepts', '?')}, OK: {fq.get('concepts_ok', '?')}")
                print(f"  Passes hard gates: {fq.get('passes_hard_gates', '?')}")
                if fq.get("hard_failures"):
                    print(f"  Hard failures: {fq['hard_failures']}")
                if fq.get("soft_warnings"):
                    print(f"  Soft warnings: {fq['soft_warnings']}")
            gtc = eval_result.get("ground_truth_comparison", {})
            if gtc:
                print(f"  Concept Jaccard: {gtc.get('concept_jaccard', '?'):.3f}")
                if "salience_delta" in gtc:
                    print(f"  Salience delta: {gtc['salience_delta']:.3f} "
                          f"(gen={gtc['gen_salience']:.2f}, gt={gtc['gt_salience']:.2f})")
            if verbose:
                # Truncate long output for readability
                text_preview = generated_text[:500]
                if len(generated_text) > 500:
                    text_preview += "..."
                print(f"  Output: {text_preview}")

    # Aggregate metrics
    n = len(results)
    if n == 0:
        return {"num_examples": 0}

    json_valid = sum(1 for r in results if r["json_valid"])
    schema_ok = sum(1 for r in results if r["schema_compliant"])
    full_schema_ok = sum(1 for r in results if r.get("full_schema_compliant", False))

    # Field quality (only for valid JSON)
    valid_results = [r for r in results if r["json_valid"]]
    summary_ok = sum(1 for r in valid_results if r["field_quality"].get("summary_ok", False))
    gist_ok = sum(1 for r in valid_results if r["field_quality"].get("gist_ok", False))
    salience_ok = sum(1 for r in valid_results if r["field_quality"].get("salience_ok", False))
    concepts_ok = sum(1 for r in valid_results if r["field_quality"].get("concepts_ok", False))
    passes_hard = sum(1 for r in valid_results if r["field_quality"].get("passes_hard_gates", False))
    nv = len(valid_results) or 1  # avoid division by zero

    # Ground truth comparison (only where available)
    jaccards = [
        r["ground_truth_comparison"]["concept_jaccard"]
        for r in results
        if r.get("ground_truth_comparison", {}).get("concept_jaccard") is not None
    ]
    salience_deltas = [
        r["ground_truth_comparison"]["salience_delta"]
        for r in results
        if r.get("ground_truth_comparison", {}).get("salience_delta") is not None
    ]

    return {
        "num_examples": n,
        "json_valid_rate": json_valid / n,
        "json_valid_count": json_valid,
        "schema_compliant_rate": schema_ok / n,
        "schema_compliant_count": schema_ok,
        "full_schema_compliant_rate": full_schema_ok / n,
        "full_schema_compliant_count": full_schema_ok,
        "summary_ok_rate": summary_ok / nv,
        "gist_ok_rate": gist_ok / nv,
        "salience_ok_rate": salience_ok / nv,
        "concepts_ok_rate": concepts_ok / nv,
        "passes_hard_gates_rate": passes_hard / nv,
        "mean_concept_jaccard": sum(jaccards) / len(jaccards) if jaccards else None,
        "mean_salience_delta": sum(salience_deltas) / len(salience_deltas) if salience_deltas else None,
        "num_with_ground_truth": len(jaccards),
        "per_example": results,
    }


# ---------------------------------------------------------------------------
# Output formatting
# ---------------------------------------------------------------------------

def print_perplexity_summary(metrics: dict) -> None:
    """Print perplexity evaluation summary."""
    print("\n" + "=" * 60)
    print("  ENCODING EVAL — PERPLEXITY")
    print("=" * 60)
    print(f"  Examples evaluated:  {metrics['num_examples']}")
    print(f"  Total tokens:        {metrics['total_tokens']}")
    print(f"  Mean loss:           {metrics['mean_loss']:.4f}")
    print(f"  Perplexity:          {metrics['ppl']:.2f}")
    if not math.isnan(metrics.get('bpb', float('nan'))):
        print(f"  Bits-per-byte:       {metrics['bpb']:.3f}")
    print()
    print("  Loss distribution:")
    print(f"    min:    {metrics['loss_min']:.4f}")
    print(f"    p25:    {metrics['loss_p25']:.4f}")
    print(f"    median: {metrics['loss_median']:.4f}")
    print(f"    p75:    {metrics['loss_p75']:.4f}")
    print(f"    max:    {metrics['loss_max']:.4f}")
    print("=" * 60)


def print_generation_summary(metrics: dict) -> None:
    """Print generation evaluation summary."""
    n = metrics["num_examples"]
    print("\n" + "=" * 60)
    print("  ENCODING EVAL — GENERATION")
    print("=" * 60)
    print(f"  Examples evaluated:        {n}")
    print()

    print("  JSON & Schema:")
    print(f"    JSON valid:              {metrics['json_valid_count']}/{n} "
          f"({metrics['json_valid_rate']:.1%})")
    print(f"    Schema compliant (min):  {metrics['schema_compliant_count']}/{n} "
          f"({metrics['schema_compliant_rate']:.1%})")
    print(f"    Schema compliant (full): {metrics['full_schema_compliant_count']}/{n} "
          f"({metrics['full_schema_compliant_rate']:.1%})")
    print()

    print("  Field quality (of valid JSON):")
    print(f"    Summary <= 100 chars:    {metrics['summary_ok_rate']:.1%}")
    print(f"    Gist <= 60 chars:        {metrics['gist_ok_rate']:.1%}")
    print(f"    Salience in [0, 1]:      {metrics['salience_ok_rate']:.1%}")
    print(f"    Concepts is list[str]:   {metrics['concepts_ok_rate']:.1%}")
    print(f"    Passes all hard gates:   {metrics['passes_hard_gates_rate']:.1%}")
    print()

    if metrics.get("mean_concept_jaccard") is not None:
        print("  Ground truth comparison:")
        print(f"    Examples with GT:        {metrics['num_with_ground_truth']}")
        print(f"    Mean concept Jaccard:    {metrics['mean_concept_jaccard']:.3f}")
        if metrics.get("mean_salience_delta") is not None:
            print(f"    Mean salience delta:     {metrics['mean_salience_delta']:.3f}")
        print()

    print("=" * 60)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Evaluate Felix-LM v3 on the mnemonic encoding task"
    )
    parser.add_argument(
        "--checkpoint", required=True,
        help="Path to model checkpoint (.pt file)",
    )
    parser.add_argument(
        "--eval-data",
        default=str(TRAINING_DIR / "data" / "finetune" / "eval.jsonl"),
        help="Path to eval JSONL (default: training/data/finetune/eval.jsonl)",
    )
    parser.add_argument(
        "--mode", choices=["perplexity", "generate"], default="perplexity",
        help="Evaluation mode (default: perplexity)",
    )
    parser.add_argument(
        "--max-tokens", type=int, default=512,
        help="Max generation length (default: 512)",
    )
    parser.add_argument(
        "--temperature", type=float, default=0.0,
        help="Sampling temperature, 0.0 for greedy (default: 0.0)",
    )
    parser.add_argument(
        "--device", type=str, default="cuda",
        help="Device (default: cuda)",
    )
    parser.add_argument(
        "--verbose", action="store_true",
        help="Print per-example details",
    )
    parser.add_argument(
        "--tokenizer",
        default=str(TRAINING_DIR / "tokenizer" / "tokenizer.json"),
        help="Path to tokenizer.json (default: training/tokenizer/tokenizer.json)",
    )
    args = parser.parse_args()

    # Validate paths
    if not Path(args.checkpoint).exists():
        print(f"Error: checkpoint not found: {args.checkpoint}")
        sys.exit(1)
    if not Path(args.eval_data).exists():
        print(f"Error: eval data not found: {args.eval_data}")
        sys.exit(1)

    device = torch.device(args.device)
    print(f"Device: {device}")
    print(f"Checkpoint: {args.checkpoint}")
    print(f"Eval data: {args.eval_data}")
    print(f"Mode: {args.mode}")

    # Load model
    print("\nLoading model...")
    model = load_model(args.checkpoint, device)
    param_count = sum(p.numel() for p in model.parameters())
    print(f"  Parameters: {param_count:,}")

    # Load eval data
    print("Loading eval data...")
    examples = load_eval_data(args.eval_data)
    print(f"  Examples: {len(examples)}")

    if not examples:
        print("Error: no valid eval examples found.")
        sys.exit(1)

    # Run evaluation
    if args.mode == "perplexity":
        print("\nRunning perplexity evaluation...")
        metrics = eval_perplexity(model, examples, device, tokenizer_path=args.tokenizer, verbose=args.verbose)
        print_perplexity_summary(metrics)

    elif args.mode == "generate":
        # Load tokenizer for decode
        if not Path(args.tokenizer).exists():
            print(f"Error: tokenizer not found: {args.tokenizer}")
            sys.exit(1)

        from tokenizers import Tokenizer
        tokenizer = Tokenizer.from_file(args.tokenizer)
        print(f"  Tokenizer vocab: {tokenizer.get_vocab_size()}")

        print("\nRunning generation evaluation...")
        metrics = eval_generate(
            model, examples, device, tokenizer,
            max_tokens=args.max_tokens,
            temperature=args.temperature,
            verbose=args.verbose,
        )
        print_generation_summary(metrics)


if __name__ == "__main__":
    main()
