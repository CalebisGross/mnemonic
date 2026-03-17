#!/usr/bin/env python3
"""Download JSON schemas and structured data for pretraining.

Collects JSON schemas from HuggingFace datasets and config file patterns
from StarCoderData to teach the model JSON fluency.

Usage:
    python download_json.py [--output-dir DIR] [--max-docs N]
"""

import argparse
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from clean import clean_document
from utils import write_jsonl

# Config file patterns to extract from StarCoderData
CONFIG_PATTERNS = {
    "package.json", "tsconfig.json", "go.mod",
    "docker-compose.yml", "docker-compose.yaml",
    "Makefile", ".github/workflows",
}


def download_schemas(output_dir: Path, max_docs: int):
    """Download JSON schemas from HuggingFace."""
    from datasets import load_dataset

    jsonl_path = output_dir / "json_schemas.jsonl"
    kept = 0

    # Try JSONSchemaBench
    print("  Streaming epfl-dlab/JSONSchemaBench...")
    try:
        ds = load_dataset("epfl-dlab/JSONSchemaBench", streaming=True, split="train")
        batch = []
        for doc in ds:
            text = doc.get("json_schema", doc.get("schema", doc.get("text", "")))
            if isinstance(text, dict):
                import json
                text = json.dumps(text, indent=2)
            if not text or len(text) < 20:
                continue

            cleaned = clean_document(text, "json_structured")
            if cleaned is None:
                continue

            batch.append({"text": cleaned, "source": "json_structured"})
            kept += 1

            if len(batch) >= 100:
                write_jsonl(jsonl_path, batch)
                batch = []

            if max_docs and kept >= max_docs:
                break

        if batch:
            write_jsonl(jsonl_path, batch)
    except Exception as e:
        print(f"  Warning: JSONSchemaBench failed: {e}")

    print(f"  Schemas: {kept:,} docs")
    return kept


def download_config_files(output_dir: Path, max_docs: int, already_have: int):
    """Extract config files (package.json, go.mod, etc.) from StarCoderData."""
    from datasets import load_dataset

    jsonl_path = output_dir / "config_files.jsonl"
    remaining = max_docs - already_have if max_docs else 50000
    if remaining <= 0:
        return 0

    print(f"  Streaming config files from StarCoderData (JSON subset)...")

    # Stream the JSON data_dir for package.json, tsconfig.json, etc.
    kept = 0
    batch = []

    for lang in ["json", "yaml"]:
        try:
            ds = load_dataset("bigcode/starcoderdata", data_dir=lang, streaming=True, split="train")
            for doc in ds:
                content = doc.get("content", "")
                if len(content) < 20 or len(content) > 50000:
                    continue

                cleaned = clean_document(content, "json_structured")
                if cleaned is None:
                    continue

                batch.append({"text": cleaned, "source": "json_structured", "language": lang})
                kept += 1

                if len(batch) >= 100:
                    write_jsonl(jsonl_path, batch)
                    batch = []

                if kept >= remaining:
                    break

        except Exception as e:
            print(f"  Warning: {lang} subset failed: {e}")

        if kept >= remaining:
            break

    if batch:
        write_jsonl(jsonl_path, batch)

    print(f"  Config files: {kept:,} docs")
    return kept


def main():
    parser = argparse.ArgumentParser(description="Download JSON/structured data for pretraining")
    parser.add_argument("--output-dir", default="data/pretrain/json_structured")
    parser.add_argument("--max-docs", type=int, default=50000)
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Downloading JSON/structured data")
    start = time.time()

    schema_count = download_schemas(output_dir, max_docs=args.max_docs // 2)
    config_count = download_config_files(output_dir, max_docs=args.max_docs, already_have=schema_count)

    elapsed = time.time() - start
    total = schema_count + config_count
    print(f"\n--- JSON Download Complete ---")
    print(f"  Total: {total:,} docs in {elapsed:.0f}s")
    print(f"  Output: {output_dir}")


if __name__ == "__main__":
    main()
