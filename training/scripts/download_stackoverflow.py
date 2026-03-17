#!/usr/bin/env python3
"""Download and filter StackOverflow posts for pretraining.

Streams mikex86/stackoverflow-posts from HuggingFace, filters by
tags and score, formats Q&A pairs, and writes to JSONL.

Usage:
    python download_stackoverflow.py [--max-docs N] [--output-dir DIR] [--min-score N]
"""

import argparse
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from clean import clean_document
from utils import write_jsonl

# Tags we want (lowercase). Posts must have at least one matching tag.
TARGET_TAGS = {
    "go", "golang",
    "python", "python-3.x",
    "javascript", "typescript", "node.js",
    "sql", "sqlite",
    "machine-learning", "deep-learning", "neural-network",
    "nlp", "natural-language-processing",
    "information-retrieval", "search", "elasticsearch", "full-text-search",
    "algorithm", "data-structures",
    "json", "rest", "api",
    "linux", "bash", "shell",
}


def parse_tags(tags) -> set[str]:
    """Parse SO tags into a set. Handles both list and '<tag1><tag2>' string formats."""
    if not tags:
        return set()
    if isinstance(tags, list):
        return {t.lower().strip() for t in tags if t}
    # String format: "<go><sqlite><json>"
    return {t.lower() for t in str(tags).replace("<", " ").replace(">", " ").split() if t}


def main():
    parser = argparse.ArgumentParser(description="Download StackOverflow posts for pretraining")
    parser.add_argument("--output-dir", default="data/pretrain/stackoverflow")
    parser.add_argument("--max-docs", type=int, default=200000)
    parser.add_argument("--min-score", type=int, default=3, help="Minimum post score")
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    jsonl_path = output_dir / "stackoverflow.jsonl"

    # Resume support
    existing = 0
    if jsonl_path.exists():
        with open(jsonl_path) as f:
            existing = sum(1 for _ in f)
        if existing >= args.max_docs:
            print(f"Already have {existing:,} docs (target: {args.max_docs:,}). Done.")
            return
        print(f"Resuming: {existing:,} docs already exist")

    from datasets import load_dataset

    print(f"Streaming mikex86/stackoverflow-posts...")
    print(f"  Min score: {args.min_score}")
    print(f"  Target tags: {len(TARGET_TAGS)} tags")
    print(f"  Max docs: {args.max_docs:,}")

    ds = load_dataset("mikex86/stackoverflow-posts", split="train", streaming=True)

    kept = 0
    seen = 0
    skipped_type = 0
    skipped_score = 0
    skipped_tags = 0
    skipped_short = 0
    batch = []
    start = time.time()

    # We want questions (PostTypeId=1) with good answers, or high-score answers
    for post in ds:
        seen += 1
        post_type = post.get("PostTypeId")
        score = post.get("Score") or 0
        body = post.get("Body") or ""
        title = post.get("Title") or ""
        tags_str = post.get("Tags") or ""

        # Only questions (type 1) and answers (type 2)
        if post_type not in (1, 2):
            skipped_type += 1
            continue

        # Score filter
        if score < args.min_score:
            skipped_score += 1
            continue

        # For questions, filter by tags
        if post_type == 1:
            tags = parse_tags(tags_str)
            if not tags & TARGET_TAGS:
                skipped_tags += 1
                continue

        # For answers, we can't easily filter by tag (no tag field on answers),
        # but high-score answers are generally useful.
        # We rely on score threshold to maintain quality.

        # Format the text
        if post_type == 1 and title:
            text = f"Question: {title}\n\n{body}"
        else:
            text = body

        if len(text) < 100:
            skipped_short += 1
            continue

        cleaned = clean_document(text, "stackoverflow")
        if cleaned is None:
            continue

        entry = {
            "text": cleaned,
            "source": "stackoverflow",
            "score": score,
            "post_type": "question" if post_type == 1 else "answer",
        }
        if tags_str and post_type == 1:
            entry["tags"] = tags_str

        batch.append(entry)

        if len(batch) >= 100:
            write_jsonl(jsonl_path, batch)
            batch = []

        kept += 1
        if kept % 10000 == 0:
            elapsed = time.time() - start
            print(f"  {kept:,} kept / {seen:,} seen ({kept / elapsed:.0f}/s)")

        if kept >= args.max_docs:
            break

    if batch:
        write_jsonl(jsonl_path, batch)

    elapsed = time.time() - start
    print(f"\n--- StackOverflow Download Complete ---")
    print(f"  Kept: {kept:,} / {seen:,}")
    print(f"  Skipped: type={skipped_type:,}, score={skipped_score:,}, "
          f"tags={skipped_tags:,}, short={skipped_short:,}")
    print(f"  Time: {elapsed:.0f}s ({kept / max(elapsed, 1):.0f}/s)")
    print(f"  Output: {jsonl_path}")


if __name__ == "__main__":
    main()
