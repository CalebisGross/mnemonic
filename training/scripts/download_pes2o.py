#!/usr/bin/env python3
"""Download and filter PeS2o v3 academic papers for pretraining.

Downloads zstandard-compressed JSONL files from allenai/peS2o v3,
filters by field of study and keyword relevance, cleans text.

Two output streams:
  - pes2o_neuro: Neuroscience, Psychology, Cognitive Science papers
  - pes2o_cs: Computer Science papers (IR, NLP, knowledge graphs)

Usage:
    python download_pes2o.py [--max-neuro N] [--max-cs N] [--output-dir DIR]
"""

import argparse
import io
import json
import re
import sys
import time
from concurrent.futures import ProcessPoolExecutor, as_completed
from pathlib import Path

import zstandard
from huggingface_hub import HfApi, hf_hub_download

sys.path.insert(0, str(Path(__file__).parent))
from clean import clean_document
from utils import write_jsonl

# --- Field of study filters ---

NEURO_FIELDS = {"neuroscience", "psychology", "cognitive science", "philosophy"}

CS_FIELDS = {"computer science"}

# --- Keyword filters (case-insensitive, matched against full text) ---
# Papers must match at least one keyword to be included.

NEURO_KEYWORDS = [
    r"memory\s+consolidation",
    r"memory\s+encoding",
    r"memory\s+retrieval",
    r"spread(?:ing)?\s+activation",
    r"episodic\s+memory",
    r"semantic\s+memory",
    r"working\s+memory",
    r"associative\s+memory",
    r"metacognition",
    r"levels?\s+of\s+processing",
    r"salience",
    r"hippocampus",
    r"hippocampal",
    r"long[\s-]term\s+potentiation",
    r"cognitive\s+load",
    r"pattern\s+recognition",
    r"schema\s+theory",
    r"memory\s+decay",
    r"memory\s+reactivation",
    r"memory\s+replay",
    r"sleep\s+(?:and\s+)?memory",
    r"attention(?:al)?\s+(?:mechanism|control|network)",
    r"neural\s+plasticity",
    r"synaptic\s+plasticity",
    r"prefrontal\s+cortex",
    r"inhibitory\s+control",
    r"cognitive\s+neuroscience",
    r"recollection",
    r"familiarity\s+(?:and\s+)?recollection",
    r"memory\s+(?:re)?consolidation",
    r"interference\s+(?:theory|in\s+memory)",
    r"forgetting\s+curve",
    r"spaced\s+repetition",
    r"desirable\s+difficult",
    r"testing\s+effect",
    r"retrieval\s+practice",
    # Consciousness studies
    r"consciousness",
    r"neural\s+correlates?\s+of\s+consciousness",
    r"global\s+workspace\s+theory",
    r"integrated\s+information\s+theory",
    r"phenomenal\s+consciousness",
    r"access\s+consciousness",
    r"qualia",
    r"self[\s-]awareness",
    r"introspection",
    r"binding\s+problem",
    r"higher[\s-]order\s+thought",
    r"predictive\s+processing",
    r"predictive\s+coding",
    r"free\s+energy\s+principle",
    r"default\s+mode\s+network",
    r"mind\s+wandering",
    r"altered\s+states?\s+of\s+consciousness",
    r"unconscious\s+processing",
    r"subliminal\s+perception",
]

CS_KEYWORDS = [
    r"information\s+retrieval",
    r"knowledge\s+graph",
    r"semantic\s+search",
    r"neural\s+network",
    r"natural\s+language\s+processing",
    r"text\s+classification",
    r"named\s+entit",
    r"concept\s+extraction",
    r"vector\s+search",
    r"embedding(?:s)?",
    r"transformer(?:s)?",
    r"language\s+model",
    r"attention\s+mechanism",
    r"graph\s+neural",
    r"retrieval[\s-]augmented",
    r"question\s+answering",
    r"text\s+summarization",
    r"sentiment\s+analysis",
    r"topic\s+model",
    r"word2vec|glove|fasttext",
    r"bert|gpt|t5\b",
]

# Compile regex patterns for speed
_neuro_re = re.compile("|".join(NEURO_KEYWORDS), re.IGNORECASE)
_cs_re = re.compile("|".join(CS_KEYWORDS), re.IGNORECASE)


def get_fields_of_study(metadata: dict) -> set[str]:
    """Extract normalized fields of study from metadata."""
    fields = set()
    for f in metadata.get("s2fieldsofstudy", []):
        fields.add(f.lower().strip())
    for f in metadata.get("extfieldsofstudy", []):
        fields.add(f.lower().strip())
    return fields


def classify_paper(doc: dict) -> str | None:
    """Classify a paper as 'neuro', 'cs', or None (skip).

    Strategy: first check field of study, then require keyword match.
    """
    metadata = doc.get("metadata", {})
    fields = get_fields_of_study(metadata)
    text = doc.get("text", "")

    # Check first 5000 chars for keyword matching (abstract + intro)
    text_head = text[:5000]

    # Neuroscience / Psychology / Cognitive Science
    if fields & NEURO_FIELDS:
        if _neuro_re.search(text_head):
            return "neuro"

    # Computer Science
    if fields & CS_FIELDS:
        if _cs_re.search(text_head):
            return "cs"

    return None


def process_shard(
    shard_file: str,
    neuro_path: Path,
    cs_path: Path,
    neuro_max: int,
    cs_max: int,
    neuro_count: int,
    cs_count: int,
) -> tuple[int, int]:
    """Process one .zst shard file and append to output JSONL files."""
    neuro_batch = []
    cs_batch = []

    dctx = zstandard.ZstdDecompressor()
    with open(shard_file, "rb") as f:
        reader = dctx.stream_reader(f)
        text_stream = io.TextIOWrapper(reader, encoding="utf-8")

        for line in text_stream:
            if neuro_count >= neuro_max and cs_count >= cs_max:
                break

            try:
                doc = json.loads(line)
            except json.JSONDecodeError:
                continue

            text = doc.get("text", "")
            if len(text) < 200:
                continue

            category = classify_paper(doc)
            if category is None:
                continue

            source = f"pes2o_{category}"
            cleaned = clean_document(text, source)
            if cleaned is None:
                continue

            metadata = doc.get("metadata", {})
            entry = {
                "text": cleaned,
                "source": source,
                "paper_id": doc.get("id"),
                "year": metadata.get("year"),
                "title": metadata.get("title", ""),
            }

            if category == "neuro" and neuro_count < neuro_max:
                neuro_batch.append(entry)
                neuro_count += 1
                if len(neuro_batch) >= 100:
                    write_jsonl(neuro_path, neuro_batch)
                    neuro_batch = []
            elif category == "cs" and cs_count < cs_max:
                cs_batch.append(entry)
                cs_count += 1
                if len(cs_batch) >= 100:
                    write_jsonl(cs_path, cs_batch)
                    cs_batch = []

    # Flush remaining
    if neuro_batch:
        write_jsonl(neuro_path, neuro_batch)
    if cs_batch:
        write_jsonl(cs_path, cs_batch)

    return neuro_count, cs_count


def _process_shard_standalone(args: tuple) -> tuple[str, list[dict], list[dict]]:
    """Process a single shard in a worker process. Returns (shard_name, neuro_docs, cs_docs).

    This is a top-level function so it can be pickled for multiprocessing.
    """
    shard_name, local_path = args

    # Re-import in worker process
    import io as _io
    import json as _json
    import re as _re
    import zstandard as _zstd

    # Recreate compiled patterns in worker
    neuro_re = _re.compile("|".join(NEURO_KEYWORDS), _re.IGNORECASE)
    cs_re = _re.compile("|".join(CS_KEYWORDS), _re.IGNORECASE)

    neuro_docs = []
    cs_docs = []

    dctx = _zstd.ZstdDecompressor()
    with open(local_path, "rb") as f:
        reader = dctx.stream_reader(f)
        text_stream = _io.TextIOWrapper(reader, encoding="utf-8")

        for line in text_stream:
            try:
                doc = _json.loads(line)
            except _json.JSONDecodeError:
                continue

            text = doc.get("text", "")
            if len(text) < 200:
                continue

            metadata = doc.get("metadata", {})
            fields = set()
            for fld in metadata.get("s2fieldsofstudy", []):
                fields.add(fld.lower().strip())
            for fld in metadata.get("extfieldsofstudy", []):
                fields.add(fld.lower().strip())

            text_head = text[:5000]
            category = None
            if fields & NEURO_FIELDS and neuro_re.search(text_head):
                category = "neuro"
            elif fields & CS_FIELDS and cs_re.search(text_head):
                category = "cs"

            if category is None:
                continue

            # Inline light cleaning (avoid importing clean module in worker)
            source = f"pes2o_{category}"
            entry = {
                "text": text,
                "source": source,
                "paper_id": doc.get("id"),
                "year": metadata.get("year"),
                "title": metadata.get("title", ""),
            }

            if category == "neuro":
                neuro_docs.append(entry)
            else:
                cs_docs.append(entry)

    return shard_name, neuro_docs, cs_docs


def _prefetch_shards(shard_names: list[str], batch_size: int = 4) -> list[tuple[str, str]]:
    """Download a batch of shards in parallel threads, return (name, local_path) pairs."""
    from concurrent.futures import ThreadPoolExecutor

    def _download(name):
        path = hf_hub_download("allenai/peS2o", name, repo_type="dataset")
        return (name, path)

    results = []
    with ThreadPoolExecutor(max_workers=batch_size) as pool:
        results = list(pool.map(_download, shard_names))
    return results


def main():
    parser = argparse.ArgumentParser(description="Download PeS2o academic papers for pretraining")
    parser.add_argument("--output-dir", default="data/pretrain")
    parser.add_argument("--max-neuro", type=int, default=500000,
                        help="Max neuroscience/psychology papers")
    parser.add_argument("--max-cs", type=int, default=200000,
                        help="Max computer science papers")
    parser.add_argument("--start-shard", type=int, default=0,
                        help="Shard index to start from (for resuming)")
    parser.add_argument("--workers", type=int, default=4,
                        help="Parallel workers for shard processing")
    parser.add_argument("--parallel", action="store_true",
                        help="Use parallel shard processing (faster)")
    args = parser.parse_args()

    neuro_dir = Path(args.output_dir) / "pes2o_neuro"
    cs_dir = Path(args.output_dir) / "pes2o_cs"
    neuro_dir.mkdir(parents=True, exist_ok=True)
    cs_dir.mkdir(parents=True, exist_ok=True)

    neuro_path = neuro_dir / "pes2o_neuro.jsonl"
    cs_path = cs_dir / "pes2o_cs.jsonl"

    # Count existing docs for resume support
    neuro_count = 0
    cs_count = 0
    if neuro_path.exists():
        with open(neuro_path) as f:
            neuro_count = sum(1 for _ in f)
        print(f"Resuming: {neuro_count:,} neuro docs already exist")
    if cs_path.exists():
        with open(cs_path) as f:
            cs_count = sum(1 for _ in f)
        print(f"Resuming: {cs_count:,} CS docs already exist")

    if neuro_count >= args.max_neuro and cs_count >= args.max_cs:
        print("Both targets already met. Nothing to do.")
        return

    api = HfApi()
    files = api.list_repo_files("allenai/peS2o", repo_type="dataset")
    shard_files = sorted(f for f in files if f.startswith("data/v3/train-") and f.endswith(".zst"))

    print(f"PeS2o v3: {len(shard_files)} shards")
    print(f"  Neuro target: {args.max_neuro:,} (have {neuro_count:,})")
    print(f"  CS target: {args.max_cs:,} (have {cs_count:,})")
    print(f"  Starting from shard {args.start_shard}")
    if args.parallel:
        print(f"  Parallel mode: {args.workers} workers")

    remaining = [s for i, s in enumerate(shard_files) if i >= args.start_shard]
    start = time.time()

    if args.parallel:
        # Parallel mode: prefetch + process batches of shards concurrently
        batch_size = args.workers
        for batch_start in range(0, len(remaining), batch_size):
            if neuro_count >= args.max_neuro and cs_count >= args.max_cs:
                break

            batch = remaining[batch_start:batch_start + batch_size]
            global_offset = args.start_shard + batch_start

            # Prefetch shards (download in parallel threads)
            print(f"\n  Prefetching shards {global_offset}-{global_offset + len(batch) - 1}...")
            shard_paths = _prefetch_shards(batch)

            # Process shards in parallel processes
            with ProcessPoolExecutor(max_workers=batch_size) as pool:
                futures = {
                    pool.submit(_process_shard_standalone, sp): sp[0]
                    for sp in shard_paths
                }

                for future in as_completed(futures):
                    shard_name, neuro_docs, cs_docs = future.result()

                    # Append results, respecting budgets
                    neuro_to_add = neuro_docs[:max(0, args.max_neuro - neuro_count)]
                    cs_to_add = cs_docs[:max(0, args.max_cs - cs_count)]

                    if neuro_to_add:
                        write_jsonl(neuro_path, neuro_to_add)
                        neuro_count += len(neuro_to_add)
                    if cs_to_add:
                        write_jsonl(cs_path, cs_to_add)
                        cs_count += len(cs_to_add)

                    elapsed = time.time() - start
                    print(f"    {shard_name}: +{len(neuro_to_add)} neuro, +{len(cs_to_add)} CS "
                          f"(total: {neuro_count:,} neuro, {cs_count:,} CS, {elapsed:.0f}s)")
    else:
        # Sequential mode (original behavior)
        for idx, shard_name in enumerate(remaining):
            global_idx = args.start_shard + idx
            if neuro_count >= args.max_neuro and cs_count >= args.max_cs:
                break

            print(f"\n  Shard {global_idx}/{len(shard_files)}: {shard_name}")
            local_path = hf_hub_download("allenai/peS2o", shard_name, repo_type="dataset")

            prev_neuro, prev_cs = neuro_count, cs_count
            neuro_count, cs_count = process_shard(
                local_path, neuro_path, cs_path,
                args.max_neuro, args.max_cs,
                neuro_count, cs_count,
            )

            shard_neuro = neuro_count - prev_neuro
            shard_cs = cs_count - prev_cs
            elapsed = time.time() - start
            print(f"    +{shard_neuro} neuro, +{shard_cs} CS "
                  f"(total: {neuro_count:,} neuro, {cs_count:,} CS, {elapsed:.0f}s)")

    elapsed = time.time() - start
    print(f"\n--- PeS2o Download Complete ---")
    print(f"  Neuroscience: {neuro_count:,} papers -> {neuro_path}")
    print(f"  CS: {cs_count:,} papers -> {cs_path}")
    print(f"  Time: {elapsed:.0f}s")


if __name__ == "__main__":
    main()
