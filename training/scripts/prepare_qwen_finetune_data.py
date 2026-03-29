#!/usr/bin/env python3
"""Prepare fine-tuning data for Qwen 3.5 2B from validated training captures.

Reads validated JSONL files, applies Qwen's chat template, tokenizes with
Qwen's tokenizer, and writes train/eval JSONL splits ready for supervised
fine-tuning with loss masking.

Usage:
    python prepare_qwen_finetune_data.py
    python prepare_qwen_finetune_data.py --max-seq-len 4096 --task-types encoding consolidation abstraction
    python prepare_qwen_finetune_data.py --include-compression ~/Projects/compressed-protocol/training_data.jsonl

Requires: pip install transformers
"""

import argparse
import json
import random
import statistics
import sys
from pathlib import Path


def get_tokenizer(tokenizer_path: str | None = None):
    """Load the Qwen 3.5 tokenizer.

    Uses a local copy if available, otherwise downloads from HuggingFace.
    """
    from transformers import AutoTokenizer

    if tokenizer_path and Path(tokenizer_path).exists():
        tok = AutoTokenizer.from_pretrained(tokenizer_path)
    else:
        tok = AutoTokenizer.from_pretrained("Qwen/Qwen3.5-2B")

    print(f"Tokenizer: vocab={tok.vocab_size}, eos={tok.eos_token}")
    return tok


def extract_messages(example: dict) -> tuple[str, str, str] | None:
    """Extract (system, user, assistant) content from a validated training example.

    Returns None if the example is malformed or missing required content.
    """
    messages = example.get("request", {}).get("messages", [])
    response_content = example.get("response", {}).get("content", "")

    if not response_content or not response_content.strip():
        return None

    system_content = ""
    user_content = ""

    for msg in messages:
        role = msg.get("role", "")
        content = msg.get("content", "")
        if role == "system":
            system_content = content
        elif role == "user":
            user_content = content

    if not user_content:
        return None

    # Strip coaching instructions and session context from encoding prompts.
    # These are 2000+ token additions designed for Gemini's large context window.
    # The fine-tuned model should learn encoding behavior from (input, output)
    # pairs, not from verbose per-prompt instructions.
    trim_markers = [
        "RECENT SESSION CONTEXT:",
        "RELATED EXISTING MEMORIES:",
        "COACHING NOTES:",
        "COACHING INSTRUCTIONS:",
    ]
    for marker in trim_markers:
        idx = user_content.find(marker)
        if idx > 0:
            user_content = user_content[:idx].rstrip()
            break

    return system_content, user_content, response_content


def apply_chat_template(tokenizer, system: str, user: str, assistant: str) -> tuple[list[int], int]:
    """Apply Qwen's chat template and return (token_ids, completion_start).

    Uses the tokenizer's built-in chat template to ensure correct formatting.
    Returns the full token sequence and the index where the assistant's
    response begins (for loss masking — only train on the completion).
    """
    # Build messages for the chat template
    messages = []
    if system:
        messages.append({"role": "system", "content": system})
    messages.append({"role": "user", "content": user})

    # Tokenize the prefix (everything before assistant response)
    prefix_text = tokenizer.apply_chat_template(
        messages, tokenize=False, add_generation_prompt=True
    )
    prefix_ids = tokenizer.encode(prefix_text, add_special_tokens=False)

    # Tokenize the full conversation (prefix + assistant response)
    messages.append({"role": "assistant", "content": assistant})
    full_text = tokenizer.apply_chat_template(
        messages, tokenize=False, add_generation_prompt=False
    )
    full_ids = tokenizer.encode(full_text, add_special_tokens=False)

    return full_ids, len(prefix_ids)


def tokenize_example(
    tokenizer,
    system: str,
    user: str,
    assistant: str,
    task_type: str,
    max_seq_len: int,
) -> dict | None:
    """Tokenize a single example, returning the output record or None if skipped."""
    full_ids, completion_start = apply_chat_template(tokenizer, system, user, assistant)

    if len(full_ids) <= max_seq_len:
        return {
            "input_ids": full_ids,
            "completion_start": completion_start,
            "seq_len": len(full_ids),
            "task_type": task_type,
        }

    # Truncate user content to fit, preserving the assistant response
    completion_len = len(full_ids) - completion_start
    max_prefix_len = max_seq_len - completion_len
    if max_prefix_len < 20:
        # Response alone is too long — skip
        return None

    # Re-tokenize with truncated user content
    user_tokens = tokenizer.encode(user, add_special_tokens=False)
    # Estimate how many user tokens to cut (rough — system/template overhead varies)
    excess = len(full_ids) - max_seq_len
    trimmed_user = tokenizer.decode(user_tokens[: len(user_tokens) - excess - 10])

    full_ids, completion_start = apply_chat_template(tokenizer, system, trimmed_user, assistant)

    if len(full_ids) > max_seq_len:
        full_ids = full_ids[:max_seq_len]

    return {
        "input_ids": full_ids,
        "completion_start": completion_start,
        "seq_len": len(full_ids),
        "task_type": task_type,
    }


def should_include(example: dict, task_types: set[str]) -> bool:
    """Determine if an example should be included based on task type filters."""
    if not example.get("parse_success", True):
        return False
    if example.get("error"):
        return False

    task_type = example.get("task_type", "unknown")

    if task_type in task_types:
        return True

    # Include "unknown" if it has encoding-like structure and we want encoding
    if task_type == "unknown" and "unknown" in task_types:
        return True

    return False


def load_validated_data(
    input_dir: Path,
    task_types: set[str],
    tokenizer,
    max_seq_len: int,
) -> tuple[list[dict], dict]:
    """Process validated JSONL files and return tokenized records."""
    jsonl_files = sorted(input_dir.glob("capture_*.jsonl"))
    if not jsonl_files:
        print(f"No capture JSONL files found in {input_dir}")
        return [], {}

    print(f"\nValidated data: {len(jsonl_files)} files")
    for f in jsonl_files:
        print(f"  {f.name}")

    records = []
    stats = {
        "total_read": 0,
        "filtered_task_type": 0,
        "filtered_extract": 0,
        "filtered_empty": 0,
        "truncated": 0,
        "skipped_too_long": 0,
    }

    for fpath in jsonl_files:
        with open(fpath) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue

                try:
                    example = json.loads(line)
                except json.JSONDecodeError:
                    continue

                stats["total_read"] += 1

                if not should_include(example, task_types):
                    stats["filtered_task_type"] += 1
                    continue

                parts = extract_messages(example)
                if parts is None:
                    stats["filtered_extract"] += 1
                    continue

                system, user, assistant = parts
                if not assistant.strip():
                    stats["filtered_empty"] += 1
                    continue

                task_type = example.get("task_type", "unknown")
                record = tokenize_example(
                    tokenizer, system, user, assistant, task_type, max_seq_len
                )
                if record is None:
                    stats["skipped_too_long"] += 1
                    continue

                records.append(record)

    return records, stats


def load_synthesis_data(
    synthesis_path: Path,
    tokenizer,
    max_seq_len: int,
) -> list[dict]:
    """Load synthesis examples (multi-turn with tool use)."""
    if not synthesis_path.exists():
        print(f"  Synthesis file not found: {synthesis_path}")
        return []

    records = []
    with open(synthesis_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue

            example = json.loads(line)
            messages = example.get("messages", [])
            if len(messages) < 2:
                continue

            # For synthesis, use the first user message and last assistant response
            system = ""
            user = ""
            assistant = ""
            for msg in messages:
                if msg["role"] == "system":
                    system = msg["content"]
                elif msg["role"] == "user" and not user:
                    user = msg["content"]
                elif msg["role"] == "assistant" and msg.get("content"):
                    assistant = msg["content"]

            if not user or not assistant:
                continue

            record = tokenize_example(
                tokenizer, system, user, assistant, "synthesis", max_seq_len
            )
            if record:
                records.append(record)

    print(f"  Synthesis: {len(records)} examples from {synthesis_path.name}")
    return records


def load_compression_data(
    compression_path: Path,
    tokenizer,
    max_seq_len: int,
    min_accuracy: float = 0.95,
    min_compression: float = 3.0,
) -> list[dict]:
    """Load compression protocol training pairs."""
    if not compression_path.exists():
        print(f"  Compression file not found: {compression_path}")
        return []

    encode_records = []
    decode_records = []

    with open(compression_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue

            example = json.loads(line)
            metadata = example.get("metadata", {})

            # Filter by quality
            accuracy = metadata.get("accuracy", 0)
            compression = metadata.get("compression_ratio", 0)
            if accuracy < min_accuracy or compression < min_compression:
                continue

            input_text = example.get("input", "")
            compressed = example.get("output", "")

            if not input_text or not compressed:
                continue

            # Encode: text -> compressed
            system = "Compress the following text into the most compact representation possible while preserving all key facts. Output only the compressed form."
            record = tokenize_example(
                tokenizer, system, input_text, compressed, "compression", max_seq_len
            )
            if record:
                encode_records.append(record)

            # Decode (reverse): compressed -> text
            system = "Decompress the following compact representation back into full natural language text, reconstructing all facts."
            record = tokenize_example(
                tokenizer, system, compressed, input_text, "decompression", max_seq_len
            )
            if record:
                decode_records.append(record)

    total = len(encode_records) + len(decode_records)
    print(f"  Compression: {len(encode_records)} encode + {len(decode_records)} decode = {total} from {compression_path.name}")
    print(f"    (filtered to acc>={min_accuracy}, comp>={min_compression})")
    return encode_records + decode_records


def verify_round_trip(tokenizer, records: list[dict], n_samples: int = 5):
    """Verify that tokenized records can be decoded back to original text."""
    print(f"\nRound-trip verification ({n_samples} samples):")
    samples = random.sample(records, min(n_samples, len(records)))
    for i, record in enumerate(samples):
        decoded = tokenizer.decode(record["input_ids"])
        prefix = tokenizer.decode(record["input_ids"][: record["completion_start"]])
        completion = tokenizer.decode(record["input_ids"][record["completion_start"] :])
        print(f"  [{i+1}] seq_len={record['seq_len']}, type={record['task_type']}")
        print(f"       prefix ends: ...{prefix[-60:]!r}")
        print(f"       completion starts: {completion[:80]!r}...")
        print()


def print_stats(records: list[dict], stats: dict, n_train: int, n_eval: int):
    """Print processing statistics."""
    seq_lens = [r["seq_len"] for r in records]
    task_counts = {}
    for r in records:
        t = r["task_type"]
        task_counts[t] = task_counts.get(t, 0) + 1

    print("\n--- Preparation Statistics ---")
    print(f"Total lines read:      {stats.get('total_read', 'N/A')}")
    print(f"Filtered (task type):  {stats.get('filtered_task_type', 'N/A')}")
    print(f"Filtered (bad format): {stats.get('filtered_extract', 'N/A')}")
    print(f"Filtered (empty resp): {stats.get('filtered_empty', 'N/A')}")
    print(f"Skipped (too long):    {stats.get('skipped_too_long', 'N/A')}")
    print(f"Truncated:             {stats.get('truncated', 'N/A')}")
    print(f"Final examples:        {len(records)}")
    print(f"  Train:               {n_train}")
    print(f"  Eval:                {n_eval}")

    print(f"\nBy task type:")
    for t, c in sorted(task_counts.items(), key=lambda x: -x[1]):
        pct = c / len(records) * 100
        print(f"  {t}: {c} ({pct:.1f}%)")

    if seq_lens:
        sorted_lens = sorted(seq_lens)
        p95_idx = int(len(sorted_lens) * 0.95)
        print(f"\nSequence lengths:")
        print(f"  Mean:   {statistics.mean(seq_lens):.0f}")
        print(f"  Median: {statistics.median(seq_lens):.0f}")
        print(f"  P95:    {sorted_lens[p95_idx]}")
        print(f"  Min:    {sorted_lens[0]}")
        print(f"  Max:    {sorted_lens[-1]}")


def main():
    parser = argparse.ArgumentParser(
        description="Prepare fine-tuning data for Qwen 3.5 2B"
    )
    parser.add_argument(
        "--input-dir",
        default="data/validated",
        help="Directory containing validated JSONL files",
    )
    parser.add_argument(
        "--output-dir",
        default="data/finetune_qwen",
        help="Output directory for train/eval JSONL",
    )
    parser.add_argument(
        "--tokenizer-path",
        default=None,
        help="Path to tokenizer directory (default: downloads Qwen/Qwen3.5-2B)",
    )
    parser.add_argument(
        "--max-seq-len",
        type=int,
        default=4096,
        help="Maximum sequence length in tokens (default: 4096)",
    )
    parser.add_argument(
        "--task-types",
        nargs="+",
        default=["encoding", "abstraction", "consolidation", "episoding", "unknown"],
        help="Task types to include from validated data",
    )
    parser.add_argument(
        "--include-synthesis",
        default=None,
        help="Path to synthesis_data.jsonl (optional)",
    )
    parser.add_argument(
        "--include-compression",
        default=None,
        help="Path to compression training_data.jsonl (optional)",
    )
    parser.add_argument(
        "--min-compression-accuracy",
        type=float,
        default=0.95,
        help="Minimum accuracy for compression pairs (default: 0.95)",
    )
    parser.add_argument(
        "--min-compression-ratio",
        type=float,
        default=3.0,
        help="Minimum compression ratio for compression pairs (default: 3.0)",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed for train/eval split",
    )
    parser.add_argument(
        "--eval-fraction",
        type=float,
        default=0.1,
        help="Fraction of data for eval set (default: 0.1)",
    )
    args = parser.parse_args()

    # Resolve paths relative to training/ directory
    script_dir = Path(__file__).resolve().parent
    training_dir = script_dir.parent

    input_dir = Path(args.input_dir)
    if not input_dir.is_absolute():
        input_dir = training_dir / input_dir

    output_dir = Path(args.output_dir)
    if not output_dir.is_absolute():
        output_dir = training_dir / output_dir

    if not input_dir.exists():
        print(f"Error: Input directory not found: {input_dir}")
        sys.exit(1)

    output_dir.mkdir(parents=True, exist_ok=True)

    task_types = set(args.task_types)
    print(f"Task types: {sorted(task_types)}")
    print(f"Max seq len: {args.max_seq_len}")

    tokenizer = get_tokenizer(args.tokenizer_path)

    # 1. Load validated training data
    all_records = []
    records, stats = load_validated_data(input_dir, task_types, tokenizer, args.max_seq_len)
    all_records.extend(records)
    print(f"  Validated: {len(records)} examples")

    # 2. Load synthesis data if provided
    if args.include_synthesis:
        synthesis_path = Path(args.include_synthesis)
        if not synthesis_path.is_absolute():
            synthesis_path = training_dir / synthesis_path
        synth_records = load_synthesis_data(synthesis_path, tokenizer, args.max_seq_len)
        all_records.extend(synth_records)

    # 3. Load compression data if provided
    if args.include_compression:
        compression_path = Path(args.include_compression)
        if not compression_path.is_absolute():
            compression_path = Path(args.include_compression)
        comp_records = load_compression_data(
            compression_path,
            tokenizer,
            args.max_seq_len,
            min_accuracy=args.min_compression_accuracy,
            min_compression=args.min_compression_ratio,
        )
        all_records.extend(comp_records)

    if not all_records:
        print("\nNo examples found. Nothing to write.")
        sys.exit(1)

    # Verify round-trip decoding
    verify_round_trip(tokenizer, all_records)

    # Shuffle and split
    random.seed(args.seed)
    random.shuffle(all_records)

    n_eval = max(1, int(len(all_records) * args.eval_fraction))
    n_train = len(all_records) - n_eval

    eval_records = all_records[:n_eval]
    train_records = all_records[n_eval:]

    # Write output files
    train_path = output_dir / "train.jsonl"
    eval_path = output_dir / "eval.jsonl"

    with open(train_path, "w") as f:
        for record in train_records:
            f.write(json.dumps(record) + "\n")

    with open(eval_path, "w") as f:
        for record in eval_records:
            f.write(json.dumps(record) + "\n")

    print_stats(all_records, stats, n_train, n_eval)
    print(f"\nOutput:")
    print(f"  Train: {train_path} ({n_train} examples)")
    print(f"  Eval:  {eval_path} ({n_eval} examples)")


if __name__ == "__main__":
    main()
