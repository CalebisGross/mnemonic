#!/usr/bin/env python3
"""Shared utilities for the pretraining data pipeline."""

import json
import time
from functools import wraps
from pathlib import Path

import numpy as np

# Shard format: raw uint16 little-endian token IDs.
# 100M tokens per shard = 200MB per shard file.
DEFAULT_SHARD_SIZE = 100_000_000  # tokens
DTYPE = np.uint16  # max value 65535, fits our 32K vocab


def retry(max_attempts=3, backoff_base=2.0, exceptions=(Exception,)):
    """Retry decorator with exponential backoff for HuggingFace streaming."""
    def decorator(fn):
        @wraps(fn)
        def wrapper(*args, **kwargs):
            for attempt in range(max_attempts):
                try:
                    return fn(*args, **kwargs)
                except exceptions as e:
                    if attempt == max_attempts - 1:
                        raise
                    wait = backoff_base ** attempt
                    print(f"  Retry {attempt + 1}/{max_attempts} after {wait:.0f}s: {e}")
                    time.sleep(wait)
        return wrapper
    return decorator


class ShardWriter:
    """Writes token IDs to .bin shard files.

    Usage:
        writer = ShardWriter("data/pretrain/tokenized/fineweb", shard_size=100_000_000)
        writer.add_tokens([101, 202, 303, ...])
        writer.add_tokens([404, 505, ...])
        writer.close()  # flushes remaining buffer
    """

    def __init__(self, output_dir: str, shard_size: int = DEFAULT_SHARD_SIZE):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.shard_size = shard_size
        self.buffer = []
        self.shard_index = 0
        self.total_tokens = 0

    @property
    def tokens_written(self) -> int:
        """Total tokens written + buffered (for accurate budget checks)."""
        return self.total_tokens + len(self.buffer)

    def add_tokens(self, tokens: list[int]):
        """Add tokens to the buffer, flushing to shard when full."""
        self.buffer.extend(tokens)
        while len(self.buffer) >= self.shard_size:
            self._flush_shard()

    def _flush_shard(self):
        """Write one shard to disk."""
        shard_tokens = self.buffer[:self.shard_size]
        self.buffer = self.buffer[self.shard_size:]

        arr = np.array(shard_tokens, dtype=DTYPE)
        path = self.output_dir / f"shard_{self.shard_index:05d}.bin"
        arr.tofile(path)

        self.total_tokens += len(shard_tokens)
        self.shard_index += 1
        print(f"  Wrote shard {self.shard_index}: {len(shard_tokens):,} tokens -> {path.name}")

    def close(self):
        """Flush remaining tokens (partial shard) and write manifest."""
        if self.buffer:
            # Write partial final shard
            arr = np.array(self.buffer, dtype=DTYPE)
            path = self.output_dir / f"shard_{self.shard_index:05d}.bin"
            arr.tofile(path)
            self.total_tokens += len(self.buffer)
            self.shard_index += 1
            print(f"  Wrote shard {self.shard_index}: {len(self.buffer):,} tokens -> {path.name}")
            self.buffer = []

        # Write manifest
        manifest = {
            "shard_count": self.shard_index,
            "total_tokens": self.total_tokens,
            "shard_size": self.shard_size,
            "dtype": "uint16",
        }
        manifest_path = self.output_dir / "manifest.json"
        with open(manifest_path, "w") as f:
            json.dump(manifest, f, indent=2)
        print(f"  Manifest: {self.total_tokens:,} tokens in {self.shard_index} shards")

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


def read_shard(path: str | Path) -> np.ndarray:
    """Read a .bin shard file as a numpy array of uint16 token IDs."""
    return np.fromfile(path, dtype=DTYPE)


def read_manifest(source_dir: str | Path) -> dict:
    """Read the manifest.json from a tokenized source directory."""
    manifest_path = Path(source_dir) / "manifest.json"
    with open(manifest_path) as f:
        return json.load(f)


def write_jsonl(path: str | Path, documents: list[dict], mode: str = "a"):
    """Append documents to a JSONL file."""
    with open(path, mode) as f:
        for doc in documents:
            f.write(json.dumps(doc, ensure_ascii=False) + "\n")


def read_jsonl(path: str | Path):
    """Yield documents from a JSONL file."""
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                yield json.loads(line)
