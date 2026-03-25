#!/usr/bin/env python3
"""Prepare fine-tuning data from validated training captures.

Reads validated JSONL files, filters for encoding examples, builds
prompt/completion pairs with chat-style delimiters, tokenizes with the
project BPE tokenizer, and writes train/eval JSONL splits ready for
supervised fine-tuning with loss masking.

Usage:
    python prepare_finetune_data.py
    python prepare_finetune_data.py --max-seq-len 2048 --task-types encoding consolidation
    python prepare_finetune_data.py --input-dir data/validated --output-dir data/finetune

Requires: pip install tokenizers
"""

import argparse
import json
import random
import statistics
import sys
from pathlib import Path


def get_tokenizer(tokenizer_path: str | None = None):
    """Load the custom BPE tokenizer.

    Follows the same pattern as tokenize_source.py.
    """
    from tokenizers import Tokenizer

    if not tokenizer_path:
        script_dir = Path(__file__).resolve().parent
        tokenizer_path = str(script_dir.parent / "tokenizer")

    tok_file = Path(tokenizer_path) / "tokenizer.json"
    if not tok_file.exists():
        print(f"Error: Tokenizer not found at {tok_file}")
        print("  Train one first: python scripts/train_tokenizer.py")
        sys.exit(1)

    tok = Tokenizer.from_file(str(tok_file))
    print(f"Tokenizer: vocab={tok.get_vocab_size()}, path={tokenizer_path}")
    return tok


def extract_messages(example: dict) -> tuple[str, str, str] | None:
    """Extract (system, user, assistant) content from a training example.

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

    return system_content, user_content, response_content


def is_encoding_like(example: dict) -> bool:
    """Check if an 'unknown' task_type example has encoding-like response structure.

    Encoding responses contain structured JSON with gist, salience, concepts, etc.
    """
    response_content = example.get("response", {}).get("content", "")
    if not response_content:
        return False

    try:
        data = json.loads(response_content)
    except (json.JSONDecodeError, TypeError):
        return False

    if not isinstance(data, dict):
        return False

    # Must have key encoding fields
    encoding_fields = {"gist", "summary", "content", "salience", "concepts"}
    return encoding_fields.issubset(data.keys())


def build_prompt(system: str, user: str, assistant: str) -> tuple[str, str]:
    """Build the full prompt text and return (prefix, completion).

    prefix = everything before the assistant's response (for loss masking)
    completion = the assistant's response + EOS token
    """
    prefix = f"<|system|>\n{system}\n<|user|>\n{user}\n<|assistant|>\n"
    completion = f"{assistant}<|endoftext|>"
    return prefix, completion


def tokenize_example(
    tokenizer,
    system: str,
    user: str,
    assistant: str,
    max_seq_len: int,
) -> dict | None:
    """Tokenize a single example, returning the output record or None if skipped.

    If the full sequence exceeds max_seq_len, the user content is trimmed
    (keeping the response intact). Returns None if trimming can't bring it
    under the limit.
    """
    prefix, completion = build_prompt(system, user, assistant)

    prefix_ids = tokenizer.encode(prefix).ids
    completion_ids = tokenizer.encode(completion).ids

    total_len = len(prefix_ids) + len(completion_ids)

    if total_len <= max_seq_len:
        input_ids = prefix_ids + completion_ids
        return {
            "input_ids": input_ids,
            "completion_start": len(prefix_ids),
            "seq_len": len(input_ids),
            "task_type": "encoding",
        }

    # Need to truncate. Only trim user content to preserve the response.
    # Compute how many tokens we can afford for the prefix.
    max_prefix_len = max_seq_len - len(completion_ids)
    if max_prefix_len < 1:
        # Response alone exceeds limit -- skip
        return None

    # Re-tokenize with trimmed user content. We tokenize the system and
    # delimiter tokens separately to find how much room user content gets.
    system_part = f"<|system|>\n{system}\n<|user|>\n"
    user_suffix = "\n<|assistant|>\n"
    system_part_ids = tokenizer.encode(system_part).ids
    user_suffix_ids = tokenizer.encode(user_suffix).ids

    overhead = len(system_part_ids) + len(user_suffix_ids)
    max_user_tokens = max_prefix_len - overhead
    if max_user_tokens < 1:
        return None

    # Tokenize user content and truncate
    user_ids = tokenizer.encode(user).ids
    if len(user_ids) > max_user_tokens:
        user_ids = user_ids[:max_user_tokens]

    # Rebuild prefix from token IDs
    prefix_ids = system_part_ids + user_ids + user_suffix_ids
    input_ids = prefix_ids + completion_ids

    # Final safety check
    if len(input_ids) > max_seq_len:
        input_ids = input_ids[:max_seq_len]

    return {
        "input_ids": input_ids,
        "completion_start": len(prefix_ids),
        "seq_len": len(input_ids),
        "task_type": "encoding",
    }


def should_include(example: dict, task_types: set[str]) -> bool:
    """Determine if an example should be included based on task type filters."""
    # Skip failed parses
    if not example.get("parse_success", True):
        return False

    # Skip errors
    if example.get("error"):
        return False

    task_type = example.get("task_type", "unknown")

    # Direct match
    if task_type in task_types:
        return True

    # Check if "unknown" examples with encoding-like structure should be included
    if task_type == "unknown" and "encoding" in task_types:
        return is_encoding_like(example)

    return False


def process_files(
    input_dir: Path,
    task_types: set[str],
    tokenizer,
    max_seq_len: int,
) -> list[dict]:
    """Process all validated JSONL files and return tokenized records."""
    jsonl_files = sorted(input_dir.glob("*.jsonl"))
    if not jsonl_files:
        print(f"No JSONL files found in {input_dir}")
        sys.exit(1)

    print(f"Input files: {len(jsonl_files)}")
    for f in jsonl_files:
        print(f"  {f.name}")

    records = []
    stats = {
        "total_read": 0,
        "filtered_task_type": 0,
        "filtered_parse": 0,
        "filtered_empty": 0,
        "filtered_extract": 0,
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
                    stats["filtered_parse"] += 1
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

                # Check for empty response content
                if not assistant.strip():
                    stats["filtered_empty"] += 1
                    continue

                # Check if truncation will be needed
                prefix, completion = build_prompt(system, user, assistant)
                prefix_ids = tokenizer.encode(prefix).ids
                completion_ids = tokenizer.encode(completion).ids
                would_truncate = (len(prefix_ids) + len(completion_ids)) > max_seq_len

                record = tokenize_example(tokenizer, system, user, assistant, max_seq_len)
                if record is None:
                    stats["skipped_too_long"] += 1
                    continue

                if would_truncate:
                    stats["truncated"] += 1

                records.append(record)

    return records, stats


def print_stats(records: list[dict], stats: dict, n_train: int, n_eval: int):
    """Print processing statistics."""
    seq_lens = [r["seq_len"] for r in records]

    print("\n--- Preparation Statistics ---")
    print(f"Total lines read:      {stats['total_read']}")
    print(f"Filtered (task type):  {stats['filtered_task_type']}")
    print(f"Filtered (parse err):  {stats['filtered_parse']}")
    print(f"Filtered (empty resp): {stats['filtered_empty']}")
    print(f"Filtered (bad format): {stats['filtered_extract']}")
    print(f"Skipped (too long):    {stats['skipped_too_long']}")
    print(f"Truncated:             {stats['truncated']}")
    print(f"Final examples:        {len(records)}")
    print(f"  Train:               {n_train}")
    print(f"  Eval:                {n_eval}")

    if seq_lens:
        print(f"\nSequence lengths:")
        print(f"  Mean:   {statistics.mean(seq_lens):.0f}")
        print(f"  Median: {statistics.median(seq_lens):.0f}")
        sorted_lens = sorted(seq_lens)
        p95_idx = int(len(sorted_lens) * 0.95)
        print(f"  P95:    {sorted_lens[p95_idx]}")
        print(f"  Min:    {sorted_lens[0]}")
        print(f"  Max:    {sorted_lens[-1]}")


def main():
    parser = argparse.ArgumentParser(
        description="Prepare fine-tuning data from validated training captures"
    )
    parser.add_argument(
        "--input-dir",
        default="data/validated",
        help="Directory containing validated JSONL files (default: data/validated)",
    )
    parser.add_argument(
        "--output-dir",
        default="data/finetune",
        help="Output directory for train/eval JSONL (default: data/finetune)",
    )
    parser.add_argument(
        "--tokenizer-path",
        default=None,
        help="Path to tokenizer directory (default: training/tokenizer/)",
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
        default=["encoding"],
        help="Task types to include (default: encoding)",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed for train/eval split (default: 42)",
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
    print(f"Eval fraction: {args.eval_fraction}")

    tokenizer = get_tokenizer(args.tokenizer_path)

    records, stats = process_files(input_dir, task_types, tokenizer, args.max_seq_len)

    if not records:
        print("\nNo examples found after filtering. Nothing to write.")
        sys.exit(1)

    # Shuffle and split
    random.seed(args.seed)
    random.shuffle(records)

    n_eval = max(1, int(len(records) * args.eval_fraction))
    n_train = len(records) - n_eval

    eval_records = records[:n_eval]
    train_records = records[n_eval:]

    # Write output files
    train_path = output_dir / "train.jsonl"
    eval_path = output_dir / "eval.jsonl"

    with open(train_path, "w") as f:
        for record in train_records:
            f.write(json.dumps(record) + "\n")

    with open(eval_path, "w") as f:
        for record in eval_records:
            f.write(json.dumps(record) + "\n")

    print_stats(records, stats, n_train, n_eval)
    print(f"\nOutput:")
    print(f"  Train: {train_path} ({n_train} examples)")
    print(f"  Eval:  {eval_path} ({n_eval} examples)")


if __name__ == "__main__":
    main()
