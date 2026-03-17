#!/usr/bin/env python3
"""Download and filter CommitPackFT for pretraining.

Downloads per-language JSONL files from bigcode/commitpackft on HF Hub.
Formats as commit message + truncated diff context.

Usage:
    python download_commits.py [--max-docs N] [--output-dir DIR]
"""

import argparse
import json
import sys
import time
from pathlib import Path

from huggingface_hub import hf_hub_download

sys.path.insert(0, str(Path(__file__).parent))
from clean import clean_document
from utils import write_jsonl

TARGET_LANGUAGES = {
    "go": "data/go/data.jsonl",
    "python": "data/python/data.jsonl",
    "javascript": "data/javascript/data.jsonl",
    "typescript": "data/typescript/data.jsonl",
    "shell": "data/shell/data.jsonl",
}


def main():
    parser = argparse.ArgumentParser(description="Download CommitPackFT for pretraining")
    parser.add_argument("--output-dir", default="data/pretrain/commits")
    parser.add_argument("--max-docs", type=int, default=100000)
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    jsonl_path = output_dir / "commits.jsonl"

    # Resume support
    existing = 0
    if jsonl_path.exists():
        with open(jsonl_path) as f:
            existing = sum(1 for _ in f)
        if existing >= args.max_docs:
            print(f"Already have {existing:,} docs (target: {args.max_docs:,}). Done.")
            return
        print(f"Resuming: {existing:,} docs already exist")

    per_lang_budget = (args.max_docs - existing) // len(TARGET_LANGUAGES)

    print(f"Downloading CommitPackFT")
    print(f"  Languages: {', '.join(TARGET_LANGUAGES.keys())}")
    print(f"  Per-language budget: ~{per_lang_budget:,}")

    kept = 0
    seen = 0
    start = time.time()

    for lang, hf_path in TARGET_LANGUAGES.items():
        print(f"\n  Downloading {lang}...")
        local_path = hf_hub_download("bigcode/commitpackft", hf_path, repo_type="dataset")

        lang_kept = 0
        batch = []

        with open(local_path) as f:
            for line in f:
                seen += 1
                try:
                    row = json.loads(line)
                except json.JSONDecodeError:
                    continue

                subject = (row.get("subject") or "").strip()
                message = (row.get("message") or "").strip()
                old_contents = row.get("old_contents") or ""
                new_contents = row.get("new_contents") or ""

                if not subject:
                    continue

                # Format as: commit message + diff context
                parts = [f"Commit: {subject}"]
                if message and message != subject:
                    parts.append(f"\n{message}")

                if old_contents and new_contents:
                    old_lines = old_contents.split("\n")[:30]
                    new_lines = new_contents.split("\n")[:30]
                    parts.append(f"\nBefore:\n" + "\n".join(old_lines))
                    parts.append(f"\nAfter:\n" + "\n".join(new_lines))
                elif new_contents:
                    new_lines = new_contents.split("\n")[:50]
                    parts.append(f"\nNew file:\n" + "\n".join(new_lines))

                text = "\n".join(parts)
                cleaned = clean_document(text, "commits")
                if cleaned is None:
                    continue

                batch.append({
                    "text": cleaned,
                    "source": "commits",
                    "language": lang,
                })

                if len(batch) >= 100:
                    write_jsonl(jsonl_path, batch)
                    batch = []

                lang_kept += 1
                kept += 1

                if lang_kept % 5000 == 0:
                    elapsed = time.time() - start
                    print(f"    {lang}: {lang_kept:,} kept ({kept:,} total, {kept / elapsed:.0f}/s)")

                if lang_kept >= per_lang_budget:
                    break

        if batch:
            write_jsonl(jsonl_path, batch)

        print(f"    {lang}: {lang_kept:,} kept")

    elapsed = time.time() - start
    print(f"\n--- CommitPack Download Complete ---")
    print(f"  Kept: {kept:,} / {seen:,}")
    print(f"  Time: {elapsed:.0f}s")
    print(f"  Output: {jsonl_path}")


if __name__ == "__main__":
    main()
