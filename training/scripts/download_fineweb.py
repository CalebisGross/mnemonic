#!/usr/bin/env python3
"""Download and filter FineWeb-Edu for pretraining.

Streams the sample-10BT config from HuggingFace, filters by education
quality score and length, cleans text, and writes to JSONL.

Usage:
    python download_fineweb.py [--max-docs N] [--output-dir DIR] [--min-score FLOAT]

Requires: pip install datasets
"""

import argparse
import sys
import time
from pathlib import Path

# Add scripts dir to path for local imports
sys.path.insert(0, str(Path(__file__).parent))
from clean import clean_document
from utils import write_jsonl


def download_fineweb(
    output_dir: str = "data/pretrain/fineweb",
    max_docs: int = 0,
    min_score: float = 3.0,
    min_length: int = 100,
    max_length: int = 50000,
    batch_size: int = 100,
):
    """Stream FineWeb-Edu and write filtered documents to JSONL."""
    from datasets import load_dataset

    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)
    jsonl_path = output_path / "fineweb_edu.jsonl"

    print(f"Streaming FineWeb-Edu (sample-10BT)...")
    print(f"  Filters: score >= {min_score}, length {min_length}-{max_length}")
    print(f"  Output: {jsonl_path}")
    if max_docs:
        print(f"  Max documents: {max_docs:,}")

    ds = load_dataset(
        "HuggingFaceFW/fineweb-edu",
        name="sample-10BT",
        streaming=True,
        split="train",
    )

    kept = 0
    seen = 0
    skipped_score = 0
    skipped_length = 0
    skipped_clean = 0
    batch = []
    start = time.time()

    for doc in ds:
        seen += 1
        text = doc.get("text", "")
        score = doc.get("score", 0)

        # Filter by education quality score
        if score < min_score:
            skipped_score += 1
            continue

        # Filter by length
        if len(text) < min_length or len(text) > max_length:
            skipped_length += 1
            continue

        # Clean
        cleaned = clean_document(text, "fineweb")
        if cleaned is None:
            skipped_clean += 1
            continue

        batch.append({
            "text": cleaned,
            "source": "fineweb",
            "score": score,
            "url": doc.get("url", ""),
        })

        if len(batch) >= batch_size:
            write_jsonl(jsonl_path, batch)
            batch = []

        kept += 1
        if kept % 10000 == 0:
            elapsed = time.time() - start
            rate = kept / elapsed
            print(f"  {kept:,} kept / {seen:,} seen ({rate:.0f} docs/s)")

        if max_docs and kept >= max_docs:
            break

    # Flush remaining
    if batch:
        write_jsonl(jsonl_path, batch)

    elapsed = time.time() - start
    print(f"\n--- FineWeb-Edu Download Complete ---")
    print(f"  Kept: {kept:,} / {seen:,} ({kept/seen*100:.1f}%)")
    print(f"  Skipped (score): {skipped_score:,}")
    print(f"  Skipped (length): {skipped_length:,}")
    print(f"  Skipped (clean): {skipped_clean:,}")
    print(f"  Time: {elapsed:.0f}s ({kept/elapsed:.0f} docs/s)")
    print(f"  Output: {jsonl_path}")


def main():
    parser = argparse.ArgumentParser(description="Download FineWeb-Edu for pretraining")
    parser.add_argument("--output-dir", default="data/pretrain/fineweb")
    parser.add_argument("--max-docs", type=int, default=0, help="0 = unlimited")
    parser.add_argument("--min-score", type=float, default=3.0)
    parser.add_argument("--min-length", type=int, default=100)
    parser.add_argument("--max-length", type=int, default=50000)
    args = parser.parse_args()

    download_fineweb(
        output_dir=args.output_dir,
        max_docs=args.max_docs,
        min_score=args.min_score,
        min_length=args.min_length,
        max_length=args.max_length,
    )


if __name__ == "__main__":
    main()
