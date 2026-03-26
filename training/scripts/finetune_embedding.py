#!/usr/bin/env python3
"""Fine-tune an embedding model on mnemonic's association graph.

Uses contrastive learning (MultipleNegativesRankingLoss) to teach the
embedding model that associated memories should have high cosine similarity.

Usage:
    source ~/Projects/felixlm/.venv/bin/activate
    python training/scripts/finetune_embedding.py \
        --base-model nomic-ai/nomic-embed-text-v2-moe \
        --data training/data/embedding_pairs.jsonl \
        --output models/mnemonic-embed-v1 \
        --epochs 5

Supports: nomic-embed-text-v2-moe (ungated, 768d MoE), EmbeddingGemma-300M
(gated, 768d), all-MiniLM-L6-v2 (ungated, 384d, fast baseline).

Requires: pip install sentence-transformers
"""

import argparse
import json
import random
from pathlib import Path

import torch
from sentence_transformers import (
    SentenceTransformer,
    SentenceTransformerTrainer,
    SentenceTransformerTrainingArguments,
    losses,
)
from sentence_transformers.evaluation import TripletEvaluator
from torch.utils.data import Dataset


class TripletDataset(Dataset):
    """Dataset of (anchor, positive, negative) text triplets."""

    def __init__(self, path: str, max_examples: int = 0, prefix: str = ""):
        self.triplets = []
        self.prefix = prefix

        with open(path) as f:
            for line in f:
                d = json.loads(line)
                self.triplets.append((
                    d["anchor"],
                    d["positive"],
                    d["negative"],
                ))
                if max_examples and len(self.triplets) >= max_examples:
                    break

    def __len__(self):
        return len(self.triplets)

    def __getitem__(self, idx):
        anchor, positive, negative = self.triplets[idx]
        if self.prefix:
            anchor = f"{self.prefix}: {anchor}"
            positive = f"{self.prefix}: {positive}"
            negative = f"{self.prefix}: {negative}"
        return {
            "anchor": anchor,
            "positive": positive,
            "negative": negative,
        }


def split_data(data_path: str, eval_ratio: float = 0.1, seed: int = 42):
    """Split triplets into train/eval."""
    with open(data_path) as f:
        lines = f.readlines()

    random.seed(seed)
    random.shuffle(lines)

    n_eval = max(1, int(len(lines) * eval_ratio))
    eval_lines = lines[:n_eval]
    train_lines = lines[n_eval:]

    train_path = data_path.replace(".jsonl", "_train.jsonl")
    eval_path = data_path.replace(".jsonl", "_eval.jsonl")

    with open(train_path, "w") as f:
        f.writelines(train_lines)
    with open(eval_path, "w") as f:
        f.writelines(eval_lines)

    return train_path, eval_path, len(train_lines), len(eval_lines)


def build_evaluator(eval_path: str, prefix: str = ""):
    """Build a TripletEvaluator from eval data."""
    anchors, positives, negatives = [], [], []
    with open(eval_path) as f:
        for line in f:
            d = json.loads(line)
            a = f"{prefix}: {d['anchor']}" if prefix else d["anchor"]
            p = f"{prefix}: {d['positive']}" if prefix else d["positive"]
            n = f"{prefix}: {d['negative']}" if prefix else d["negative"]
            anchors.append(a)
            positives.append(p)
            negatives.append(n)

    return TripletEvaluator(
        anchors=anchors,
        positives=positives,
        negatives=negatives,
        name="mnemonic-associations",
    )


# Model-specific prefixes (some models require task prefixes)
MODEL_PREFIXES = {
    "nomic-ai/nomic-embed-text-v2-moe": "search_document",
    "nomic-ai/nomic-embed-text-v1.5": "search_document",
    "google/embeddinggemma-300m": "",
    "all-MiniLM-L6-v2": "",
}


def main():
    parser = argparse.ArgumentParser(
        description="Fine-tune embedding model on mnemonic association pairs"
    )
    parser.add_argument(
        "--base-model", type=str,
        default="nomic-ai/nomic-embed-text-v2-moe",
        help="Base embedding model (HuggingFace model ID)",
    )
    parser.add_argument(
        "--data", type=str,
        default="training/data/embedding_pairs.jsonl",
        help="Path to triplet JSONL data",
    )
    parser.add_argument(
        "--output", type=str,
        default="models/mnemonic-embed-v1",
        help="Output directory for fine-tuned model",
    )
    parser.add_argument("--epochs", type=int, default=5)
    parser.add_argument("--batch-size", type=int, default=32)
    parser.add_argument("--lr", type=float, default=2e-5)
    parser.add_argument("--warmup-ratio", type=float, default=0.1)
    parser.add_argument("--max-examples", type=int, default=0, help="0 = use all")
    parser.add_argument("--eval-ratio", type=float, default=0.05)
    parser.add_argument("--matryoshka-dims", type=str, default="768,512,384,256",
                        help="Comma-separated Matryoshka dimensions")
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--fp16", action="store_true", default=True)
    parser.add_argument("--trust-remote-code", action="store_true", default=True)
    args = parser.parse_args()

    random.seed(args.seed)
    torch.manual_seed(args.seed)

    # Determine task prefix
    prefix = MODEL_PREFIXES.get(args.base_model, "")
    if prefix:
        print(f"Using task prefix: '{prefix}'")

    # Split data
    print(f"Splitting data from {args.data}...")
    train_path, eval_path, n_train, n_eval = split_data(
        args.data, args.eval_ratio, args.seed
    )
    print(f"  Train: {n_train}, Eval: {n_eval}")

    # Load model
    print(f"\nLoading base model: {args.base_model}...")
    model = SentenceTransformer(
        args.base_model,
        trust_remote_code=args.trust_remote_code,
    )
    print(f"  Embedding dim: {model.get_sentence_embedding_dimension()}")
    print(f"  Max seq length: {model.max_seq_length}")

    # Load datasets
    train_dataset = TripletDataset(train_path, args.max_examples, prefix)
    print(f"  Train dataset: {len(train_dataset)} triplets")

    # Build evaluator
    evaluator = build_evaluator(eval_path, prefix)

    # Loss: MultipleNegativesRankingLoss with Matryoshka
    matryoshka_dims = [int(d) for d in args.matryoshka_dims.split(",")]
    print(f"  Matryoshka dims: {matryoshka_dims}")

    inner_loss = losses.MultipleNegativesRankingLoss(model)
    train_loss = losses.MatryoshkaLoss(model, inner_loss, matryoshka_dims)

    # Training args
    training_args = SentenceTransformerTrainingArguments(
        output_dir=args.output,
        num_train_epochs=args.epochs,
        per_device_train_batch_size=args.batch_size,
        learning_rate=args.lr,
        warmup_ratio=args.warmup_ratio,
        fp16=args.fp16,
        eval_strategy="epoch",
        save_strategy="epoch",
        logging_steps=50,
        seed=args.seed,
        load_best_model_at_end=True,
        metric_for_best_model="mnemonic-associations_cosine_accuracy",
    )

    # Train
    trainer = SentenceTransformerTrainer(
        model=model,
        args=training_args,
        train_dataset=train_dataset,
        eval_dataset=None,
        loss=train_loss,
        evaluator=evaluator,
    )

    print(f"\nStarting training...")
    print(f"  Epochs: {args.epochs}")
    print(f"  Batch size: {args.batch_size}")
    print(f"  LR: {args.lr}")
    print(f"  Steps/epoch: {len(train_dataset) // args.batch_size}")
    print(f"  Total steps: {len(train_dataset) // args.batch_size * args.epochs}")

    trainer.train()

    # Save final model
    print(f"\nSaving model to {args.output}...")
    model.save_pretrained(args.output)

    # Final evaluation
    print("\nFinal evaluation:")
    results = evaluator(model)
    for k, v in results.items():
        print(f"  {k}: {v:.4f}")

    print(f"\n=== Training Complete ===")
    print(f"  Model saved to: {args.output}")
    print(f"  To use with mnemonic: load via sentence-transformers or convert to GGUF")


if __name__ == "__main__":
    main()
