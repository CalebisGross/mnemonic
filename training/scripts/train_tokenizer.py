#!/usr/bin/env python3
"""Train a custom BPE tokenizer on the mnemonic data mix.

Samples proportionally from each data source, trains a BPE tokenizer
with 32K vocab, and validates coverage on domain-specific terms.

Usage:
    python train_tokenizer.py [--output-dir DIR] [--vocab-size N] [--sample-size N]

Requires: pip install tokenizers
"""

import argparse
import json
import random
import sys
import time
from pathlib import Path

from tokenizers import Tokenizer, models, trainers, pre_tokenizers, decoders, normalizers

sys.path.insert(0, str(Path(__file__).parent))

# Source weights from pretrain_mix.yaml (proportional sampling)
SOURCE_WEIGHTS = {
    "pes2o_neuro": 0.22,
    "pes2o_cs": 0.10,
    "code": 0.28,
    "fineweb": 0.18,
    "stackoverflow": 0.08,
    "json_structured": 0.05,
    "commits": 0.05,
}

# Domain terms that MUST tokenize well (single or few tokens)
DOMAIN_TERMS = {
    "neuroscience": [
        "hippocampus", "consolidation", "episodic", "metacognition",
        "salience", "potentiation", "synaptic", "plasticity",
        "amygdala", "prefrontal", "encoding", "retrieval",
        "consciousness", "qualia", "phenomenal",
    ],
    "code": [
        "func", "struct", "interface", "goroutine", "channel",
        "import", "return", "async", "await", "lambda",
        "def", "class", "self", "None", "True", "False",
    ],
    "json": [
        '{"', '"}', '":', '",', "null", "true", "false",
        "\\n", "\\t",
    ],
    "mnemonic": [
        "mnemonic", "memory", "encoding", "consolidation",
        "retrieval", "abstraction", "perception", "dreaming",
        "orchestrator", "reactor", "spread activation",
    ],
}

SPECIAL_TOKENS = ["<|endoftext|>", "<|pad|>"]


def sample_texts(data_dir: Path, total_chars: int) -> list[str]:
    """Sample text proportionally from each source using reservoir sampling.

    Streams through files without loading them into memory.
    Uses reservoir sampling to get a random subset within the char budget.
    """
    texts = []

    for source, weight in SOURCE_WEIGHTS.items():
        source_dir = data_dir / source
        if not source_dir.exists():
            print(f"  Skipping {source} (not downloaded yet)")
            continue

        jsonl_files = sorted(source_dir.glob("*.jsonl"))
        if not jsonl_files:
            print(f"  Skipping {source} (no JSONL files)")
            continue

        budget = int(total_chars * weight)
        collected = 0
        source_texts = []

        # Stream through files, reservoir-sample within budget.
        # First pass: take every Nth line to stay under budget without
        # loading everything into memory.
        # We estimate total lines from file sizes (~200 bytes/line avg).
        total_bytes = sum(jf.stat().st_size for jf in jsonl_files)
        est_lines = total_bytes // 200
        # Target ~2x budget in chars, then truncate (avg doc ~500 chars)
        est_needed = (budget * 2) // 500
        skip_rate = max(1, est_lines // max(est_needed, 1))

        rng = random.Random(42)
        line_idx = 0
        for jf in jsonl_files:
            with open(jf) as f:
                for line in f:
                    line_idx += 1
                    # Deterministic sampling: take every Nth line with jitter
                    if rng.randint(1, skip_rate) != 1:
                        continue
                    if collected >= budget:
                        break
                    try:
                        doc = json.loads(line)
                        text = doc.get("text", "")
                        if text:
                            source_texts.append(text)
                            collected += len(text)
                    except json.JSONDecodeError:
                        continue
            if collected >= budget:
                break

        texts.extend(source_texts)
        print(f"  {source}: {len(source_texts):,} docs, {collected:,} chars ({collected / 1e6:.0f} MB)")

    random.shuffle(texts)
    return texts


def train_tokenizer(texts: list[str], vocab_size: int) -> Tokenizer:
    """Train a BPE tokenizer on the given texts."""
    tokenizer = Tokenizer(models.BPE())

    tokenizer.normalizer = normalizers.Sequence([
        normalizers.NFC(),
        normalizers.Replace("\r\n", "\n"),
        normalizers.Replace("\r", "\n"),
    ])

    # Byte-level pre-tokenizer (like GPT-2) — handles any byte sequence
    tokenizer.pre_tokenizer = pre_tokenizers.ByteLevel(add_prefix_space=False)
    tokenizer.decoder = decoders.ByteLevel()

    trainer = trainers.BpeTrainer(
        vocab_size=vocab_size,
        special_tokens=SPECIAL_TOKENS,
        min_frequency=2,
        show_progress=True,
        initial_alphabet=pre_tokenizers.ByteLevel.alphabet(),
    )

    # Train from iterator (memory efficient)
    print(f"\nTraining BPE tokenizer (vocab_size={vocab_size:,})...")
    start = time.time()
    tokenizer.train_from_iterator(texts, trainer=trainer)
    elapsed = time.time() - start
    print(f"  Training took {elapsed:.0f}s")
    print(f"  Final vocab size: {tokenizer.get_vocab_size():,}")

    return tokenizer


def evaluate_tokenizer(tokenizer: Tokenizer):
    """Evaluate tokenizer on domain-specific terms and compare to GPT-2."""
    print("\n--- Domain Term Coverage ---")

    for category, terms in DOMAIN_TERMS.items():
        print(f"\n  {category}:")
        for term in terms:
            encoded = tokenizer.encode(term)
            tokens = encoded.tokens
            n_tokens = len(tokens)
            marker = "  " if n_tokens <= 2 else "!!"
            print(f"    {marker} '{term}' -> {n_tokens} tokens: {tokens}")

    # Compare tokens/char ratio on sample texts
    print("\n--- Efficiency Comparison ---")
    sample_texts_by_domain = {
        "neuroscience": "The hippocampus plays a critical role in memory consolidation during sleep. "
                        "Episodic memories are gradually transferred to neocortical stores through "
                        "synaptic plasticity mechanisms including long-term potentiation.",
        "go_code": 'func (s *Store) GetMemory(ctx context.Context, id string) (*Memory, error) {\n'
                   '\trow := s.db.QueryRowContext(ctx, "SELECT id, text, source FROM memories WHERE id = ?", id)\n'
                   '\tvar m Memory\n'
                   '\tif err := row.Scan(&m.ID, &m.Text, &m.Source); err != nil {\n'
                   '\t\treturn nil, fmt.Errorf("getting memory %s: %w", id, err)\n'
                   '\t}\n'
                   '\treturn &m, nil\n'
                   '}',
        "json": '{"type": "encoding", "content": {"text": "Memory consolidation occurs during sleep", '
                '"salience": 0.85, "concepts": ["memory", "consolidation", "sleep"]}}',
    }

    try:
        from transformers import AutoTokenizer
        gpt2_tok = AutoTokenizer.from_pretrained("gpt2")
        has_gpt2 = True
    except Exception:
        has_gpt2 = False

    for domain, text in sample_texts_by_domain.items():
        custom_ids = tokenizer.encode(text).ids
        custom_ratio = len(custom_ids) / len(text)
        line = f"  {domain}: custom={len(custom_ids)} tokens ({custom_ratio:.3f} tok/char)"

        if has_gpt2:
            gpt2_ids = gpt2_tok.encode(text)
            gpt2_ratio = len(gpt2_ids) / len(text)
            improvement = (1 - custom_ratio / gpt2_ratio) * 100
            line += f", gpt2={len(gpt2_ids)} ({gpt2_ratio:.3f} tok/char), {improvement:+.1f}%"

        print(line)


def main():
    parser = argparse.ArgumentParser(description="Train custom BPE tokenizer for mnemonic-LM")
    parser.add_argument("--output-dir", default="training/tokenizer")
    parser.add_argument("--data-dir", default="data/pretrain")
    parser.add_argument("--vocab-size", type=int, default=32768)
    parser.add_argument("--sample-size", type=int, default=1_000_000_000,
                        help="Total chars to sample for training (default: 1GB)")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    random.seed(args.seed)
    data_dir = Path(args.data_dir)
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Sampling training data from {data_dir}")
    print(f"  Target: {args.sample_size:,} chars (~{args.sample_size / 1e9:.1f} GB)")
    texts = sample_texts(data_dir, args.sample_size)

    if not texts:
        print("Error: No training data found. Download data first.")
        sys.exit(1)

    total_chars = sum(len(t) for t in texts)
    print(f"\n  Total: {len(texts):,} docs, {total_chars:,} chars ({total_chars / 1e9:.2f} GB)")

    tokenizer = train_tokenizer(texts, args.vocab_size)

    # Save
    tokenizer.save(str(output_dir / "tokenizer.json"))

    # Write config for reference
    config = {
        "vocab_size": tokenizer.get_vocab_size(),
        "special_tokens": SPECIAL_TOKENS,
        "eos_token_id": tokenizer.get_vocab_size() - 1,
        "pad_token_id": tokenizer.token_to_id("<|pad|>"),
        "training_chars": total_chars,
        "training_docs": len(texts),
    }
    with open(output_dir / "config.json", "w") as f:
        json.dump(config, f, indent=2)

    print(f"\nSaved tokenizer to {output_dir}/")

    # Evaluate
    evaluate_tokenizer(tokenizer)

    print(f"\n--- Done ---")
    print(f"  Tokenizer: {output_dir}/tokenizer.json")
    print(f"  Config: {output_dir}/config.json")
    print(f"  Vocab size: {tokenizer.get_vocab_size():,}")


if __name__ == "__main__":
    main()
