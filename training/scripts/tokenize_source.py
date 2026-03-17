#!/usr/bin/env python3
"""Tokenize cleaned JSONL files into .bin shard files for training.

Reads cleaned JSONL from a source directory, tokenizes with the project
tokenizer (custom BPE or GPT-2 fallback), inserts EOS between documents,
and writes packed uint16 token shards.

Usage:
    python tokenize_source.py --source fineweb [--tokenizer-path PATH] [--shard-size N]
    python tokenize_source.py --source code --tokenizer-path training/tokenizer
    python tokenize_source.py --source pes2o_neuro --max-tokens 2200000000

Requires: pip install tokenizers transformers
"""

import argparse
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from utils import ShardWriter, read_jsonl


def get_tokenizer(tokenizer_path: str | None = None):
    """Load the tokenizer. Custom BPE if available, GPT-2 fallback."""
    if tokenizer_path and Path(tokenizer_path).exists():
        from tokenizers import Tokenizer
        tok = Tokenizer.from_file(str(Path(tokenizer_path) / "tokenizer.json"))
        print(f"Using custom tokenizer from {tokenizer_path} (vocab={tok.get_vocab_size()})")
        return tok, tok.get_vocab_size() - 1  # EOS = last token
    else:
        from transformers import AutoTokenizer
        tok = AutoTokenizer.from_pretrained("gpt2")
        print(f"Using GPT-2 tokenizer (vocab={tok.vocab_size})")
        return tok, tok.eos_token_id


def tokenize_text(tokenizer, text: str) -> list[int]:
    """Tokenize text, handling both HF tokenizer and raw tokenizers lib."""
    if hasattr(tokenizer, "encode_batch"):
        # tokenizers library Tokenizer
        return tokenizer.encode(text).ids
    else:
        # HuggingFace transformers AutoTokenizer
        return tokenizer.encode(text)


def tokenize_source(
    source: str,
    data_dir: str = "data/pretrain",
    output_dir: str = "data/pretrain/tokenized",
    tokenizer_path: str | None = None,
    shard_size: int = 100_000_000,
    max_tokens: int = 0,
):
    """Tokenize a source's JSONL files into .bin shards."""
    source_dir = Path(data_dir) / source
    out_dir = Path(output_dir) / source

    if not source_dir.exists():
        print(f"Error: Source directory not found: {source_dir}")
        sys.exit(1)

    jsonl_files = sorted(source_dir.glob("*.jsonl"))
    if not jsonl_files:
        print(f"Error: No JSONL files found in {source_dir}")
        sys.exit(1)

    print(f"Tokenizing source '{source}'")
    print(f"  Input: {source_dir} ({len(jsonl_files)} files)")
    print(f"  Output: {out_dir}")
    if max_tokens:
        print(f"  Token budget: {max_tokens:,}")

    tokenizer, eos_id = get_tokenizer(tokenizer_path)
    start = time.time()
    docs_processed = 0
    docs_skipped = 0

    with ShardWriter(str(out_dir), shard_size=shard_size) as writer:
        for jsonl_file in jsonl_files:
            print(f"  Processing {jsonl_file.name}...")

            for doc in read_jsonl(jsonl_file):
                text = doc.get("text", "")
                if not text:
                    docs_skipped += 1
                    continue

                tokens = tokenize_text(tokenizer, text)
                if not tokens:
                    docs_skipped += 1
                    continue

                # Add EOS separator between documents
                tokens.append(eos_id)
                writer.add_tokens(tokens)
                docs_processed += 1

                if max_tokens and writer.tokens_written >= max_tokens:
                    print(f"  Reached token budget ({max_tokens:,})")
                    break

                if docs_processed % 50000 == 0:
                    elapsed = time.time() - start
                    print(f"  {docs_processed:,} docs, {writer.tokens_written:,} tokens ({elapsed:.0f}s)")

            if max_tokens and writer.tokens_written >= max_tokens:
                break

    elapsed = time.time() - start
    print(f"\n--- Tokenization Complete ---")
    print(f"  Source: {source}")
    print(f"  Documents: {docs_processed:,} processed, {docs_skipped:,} skipped")
    print(f"  Tokens: {writer.tokens_written:,}")
    print(f"  Shards: {writer.shard_index}")
    print(f"  Time: {elapsed:.0f}s")


def main():
    parser = argparse.ArgumentParser(description="Tokenize cleaned JSONL into .bin shards")
    parser.add_argument("--source", required=True, help="Source name (fineweb, code, pes2o_neuro, etc.)")
    parser.add_argument("--data-dir", default="data/pretrain")
    parser.add_argument("--output-dir", default="data/pretrain/tokenized")
    parser.add_argument("--tokenizer-path", default=None, help="Path to custom tokenizer dir")
    parser.add_argument("--shard-size", type=int, default=100_000_000)
    parser.add_argument("--max-tokens", type=int, default=0, help="0 = unlimited")
    args = parser.parse_args()

    tokenize_source(
        source=args.source,
        data_dir=args.data_dir,
        output_dir=args.output_dir,
        tokenizer_path=args.tokenizer_path,
        shard_size=args.shard_size,
        max_tokens=args.max_tokens,
    )


if __name__ == "__main__":
    main()
