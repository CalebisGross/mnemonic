#!/usr/bin/env python3
"""Fine-tune Felix-LM v3 on mnemonic encoding task data.

Loads a pretrained checkpoint, freezes the backbone, and fine-tunes
only the spoke parameters (and optionally output_norm + embed_proj)
on encoding-task JSONL data produced by prepare_finetune_data.py.

Usage:
    python training/scripts/finetune_mnemonic.py --pretrained checkpoints/v3_mnemonic_100m/last.pt
    python training/scripts/finetune_mnemonic.py --pretrained checkpoints/v3_mnemonic_100m/last.pt --smoke-test
    python training/scripts/finetune_mnemonic.py --pretrained checkpoints/v3_mnemonic_100m/last.pt --unfreeze-norm

Requires:
    - Felix-LM installed/importable: pip install -e ~/Projects/felixlm
    - JSONL data in training/data/finetune/ (train.jsonl, eval.jsonl)
"""

import argparse
import json
import math
import sys
import time
from pathlib import Path

import torch
import torch.nn.functional as F
from torch.utils.data import DataLoader, Dataset

# Add mnemonic training scripts to path
TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))


# --- Mnemonic-specific config (same as pretraining) ---

def make_v3_mnemonic_100m_config():
    """Felix-LM v3 at ~100M params with mnemonic's 32K vocab."""
    from felix_lm.v3.config import FelixV3Config

    return FelixV3Config(
        vocab_size=32768,
        d_embed=512,
        num_layers=20,
        num_heads=8,
        num_spokes=4,
        spoke_rank=64,
        gate_schedule="uniform",
        embed_proj=True,
        dropout=0.1,
        gradient_checkpointing=True,
    )


# --- Dataset ---

class FineTuneDataset(Dataset):
    """Dataset for encoding-task fine-tuning.

    Loads JSONL files where each line has:
        {"input_ids": [...], "completion_start": N, "seq_len": M}

    Returns (input_ids, targets, loss_mask) where:
        - targets = input_ids shifted left by 1 (standard LM)
        - loss_mask = 0 before completion_start, 1 at and after
    """

    def __init__(self, path: str, max_seq_len: int, pad_token_id: int = 0):
        self.max_seq_len = max_seq_len
        self.pad_token_id = pad_token_id
        self.samples = []

        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                self.samples.append(json.loads(line))

        print(f"  Loaded {len(self.samples)} samples from {path}")

    def __len__(self):
        return len(self.samples)

    def __getitem__(self, idx):
        sample = self.samples[idx]
        input_ids = sample["input_ids"]
        completion_start = sample["completion_start"]
        seq_len = sample.get("seq_len", len(input_ids))

        # Truncate to max_seq_len
        if len(input_ids) > self.max_seq_len:
            input_ids = input_ids[: self.max_seq_len]
            seq_len = self.max_seq_len

        # Build targets: shifted left by 1
        # For position i, target is input_ids[i+1]
        # Last position has no target, pad with pad_token_id
        targets = input_ids[1:] + [self.pad_token_id]

        # Build loss mask: 0 before completion_start, 1 at and after
        # Applied to the targets, so position i predicts token i+1
        # We want loss on predictions starting from completion_start
        loss_mask = [0.0] * min(completion_start, len(input_ids)) + [1.0] * max(
            0, len(input_ids) - completion_start
        )

        # Mask out the last position (has no real target)
        if len(loss_mask) > 0:
            loss_mask[-1] = 0.0

        # Pad to max_seq_len
        pad_len = self.max_seq_len - len(input_ids)
        input_ids = input_ids + [self.pad_token_id] * pad_len
        targets = targets + [self.pad_token_id] * pad_len
        loss_mask = loss_mask + [0.0] * pad_len

        return (
            torch.tensor(input_ids, dtype=torch.long),
            torch.tensor(targets, dtype=torch.long),
            torch.tensor(loss_mask, dtype=torch.float32),
        )


# --- LR Schedule ---

def get_lr(
    step: int, warmup_steps: int, max_steps: int, max_lr: float, min_lr: float
) -> float:
    """Cosine learning rate schedule with linear warmup."""
    if step < warmup_steps:
        return max_lr * step / warmup_steps
    if step >= max_steps:
        return min_lr
    progress = (step - warmup_steps) / (max_steps - warmup_steps)
    return min_lr + 0.5 * (max_lr - min_lr) * (1 + math.cos(math.pi * progress))


# --- Eval ---

def evaluate(model, eval_loader, device, autocast_ctx):
    """Run evaluation on the eval set, return average masked loss."""
    model.eval()
    total_loss = 0.0
    total_tokens = 0

    with torch.no_grad():
        for input_ids, targets, loss_mask in eval_loader:
            input_ids = input_ids.to(device)
            targets = targets.to(device)
            loss_mask = loss_mask.to(device)

            with autocast_ctx:
                result = model(input_ids)
                logits = result["logits"]

                # Flatten for cross_entropy
                B, T, V = logits.shape
                per_token_loss = F.cross_entropy(
                    logits.view(B * T, V), targets.view(B * T), reduction="none"
                )
                per_token_loss = per_token_loss.view(B, T)

                masked_loss = (per_token_loss * loss_mask).sum()
                mask_count = loss_mask.sum()

            total_loss += masked_loss.item()
            total_tokens += mask_count.item()

    model.train()
    if total_tokens == 0:
        return 0.0
    return total_loss / total_tokens


# --- Training ---

def train(config, args):
    from felix_lm.v3.model import FelixLMv3
    from felix_lm.utils import count_parameters

    device = torch.device(args.device)

    # Build model
    model = FelixLMv3(config).to(device)

    # Load pretrained checkpoint
    print(f"\nLoading pretrained checkpoint: {args.pretrained}")
    ckpt = torch.load(args.pretrained, map_location=device, weights_only=False)

    if isinstance(ckpt, dict) and "model_state_dict" in ckpt:
        state_dict = ckpt["model_state_dict"]
    else:
        state_dict = ckpt

    # Strip _orig_mod. prefix if present (saved from torch.compile'd model)
    if any(k.startswith("_orig_mod.") for k in state_dict.keys()):
        state_dict = {
            k.replace("_orig_mod.", "", 1): v for k, v in state_dict.items()
        }

    model.load_state_dict(state_dict)
    print("  Checkpoint loaded successfully")

    # Freeze/unfreeze strategy
    trainable_params = []

    if args.full_finetune:
        # Full fine-tune: all parameters trainable
        for param in model.parameters():
            param.requires_grad_(True)
            trainable_params.append(param)
        print("  Unfroze: ALL parameters (full fine-tune)")
    else:
        # Freeze everything first
        model.requires_grad_(False)

        # Always unfreeze spokes
        if model.spokes is not None:
            for param in model.spokes.parameters():
                param.requires_grad_(True)
                trainable_params.append(param)

        # Optionally unfreeze last N transformer layers
        if args.unfreeze_layers > 0:
            num_layers = len(model.layers)
            start_layer = max(0, num_layers - args.unfreeze_layers)
            for layer_idx in range(start_layer, num_layers):
                for param in model.layers[layer_idx].parameters():
                    param.requires_grad_(True)
                    trainable_params.append(param)
            print(f"  Unfroze: spokes + layers {start_layer}-{num_layers - 1}")

        if args.unfreeze_norm:
            if model.output_norm is not None:
                for param in model.output_norm.parameters():
                    param.requires_grad_(True)
                    trainable_params.append(param)
            if model.embed_proj is not None:
                for param in model.embed_proj.parameters():
                    param.requires_grad_(True)
                    trainable_params.append(param)

        if not args.unfreeze_layers and not args.unfreeze_norm:
            print("  Unfroze: spokes only")
        elif args.unfreeze_norm and not args.unfreeze_layers:
            print("  Unfroze: spokes, output_norm, embed_proj")
        elif args.unfreeze_norm and args.unfreeze_layers:
            print(f"  Unfroze: spokes, last {args.unfreeze_layers} layers, output_norm, embed_proj")

    total_params = sum(p.numel() for p in model.parameters())
    trainable_count = sum(p.numel() for p in model.parameters() if p.requires_grad)
    frozen_count = total_params - trainable_count
    print(
        f"  Trainable: {trainable_count:,} / {total_params:,} "
        f"({100 * trainable_count / total_params:.1f}%)"
    )
    print(f"  Frozen: {frozen_count:,}")

    # Compile after freeze/unfreeze
    if args.compile:
        print("  Compiling model with torch.compile...")
        model = torch.compile(model)

    # Mixed precision
    if args.dtype == "fp32":
        autocast_ctx = torch.autocast("cuda", enabled=False)
        print("  Precision: fp32")
    elif device.type == "cuda":
        autocast_ctx = torch.autocast("cuda", dtype=torch.bfloat16)
        print("  Mixed precision: bf16")
    else:
        autocast_ctx = torch.autocast("cpu", enabled=False)

    # Data
    train_ds = FineTuneDataset(args.train_data, max_seq_len=args.seq_len)
    eval_ds = FineTuneDataset(args.eval_data, max_seq_len=args.seq_len)

    train_loader = DataLoader(
        train_ds,
        batch_size=args.batch_size,
        shuffle=True,
        num_workers=0,
        pin_memory=True,
        drop_last=True,
    )
    eval_loader = DataLoader(
        eval_ds,
        batch_size=args.batch_size,
        shuffle=False,
        num_workers=0,
        pin_memory=True,
    )

    # Compute steps
    steps_per_epoch = len(train_loader)
    total_micro_steps = steps_per_epoch * args.epochs
    opt_steps = total_micro_steps // args.grad_accum
    print(f"\nData: {len(train_ds)} train, {len(eval_ds)} eval samples")
    print(f"  Steps/epoch: {steps_per_epoch}, epochs: {args.epochs}")
    print(f"  Total micro-steps: {total_micro_steps}, optimizer steps: {opt_steps}")
    print(
        f"  grad_accum={args.grad_accum}, effective batch={args.batch_size * args.grad_accum}"
    )

    # Auto-scale warmup
    if args.warmup_steps == 0:
        args.warmup_steps = max(1, opt_steps // 10)
    print(f"  Warmup: {args.warmup_steps} optimizer steps")

    # Optimizer (only trainable params)
    optimizer = torch.optim.AdamW(
        trainable_params,
        lr=args.lr,
        weight_decay=args.weight_decay,
        betas=(0.9, 0.95),
    )

    # wandb
    if not args.no_wandb:
        import wandb

        run_name = args.wandb_name or "finetune_encoding"
        wandb.init(
            project="mnemonic-lm",
            name=run_name,
            config={
                "task": "finetune",
                "total_params": total_params,
                "trainable_params": trainable_count,
                "lr": args.lr,
                "batch_size": args.batch_size,
                "grad_accum": args.grad_accum,
                "seq_len": args.seq_len,
                "epochs": args.epochs,
                "opt_steps": opt_steps,
                "unfreeze_norm": args.unfreeze_norm,
                "unfreeze_layers": args.unfreeze_layers,
                "full_finetune": args.full_finetune,
                "pretrained": args.pretrained,
            },
        )

    # Checkpoint dir
    ckpt_dir = Path("checkpoints/v3_mnemonic_100m_ft")
    ckpt_dir.mkdir(parents=True, exist_ok=True)

    # Training loop
    global_step = 0
    opt_step_count = 0
    lr = args.lr
    losses = []

    model.train()
    optimizer.zero_grad()
    start_time = time.time()

    try:
        from tqdm import tqdm
    except ImportError:
        tqdm = None

    for epoch in range(args.epochs):
        if tqdm is not None:
            pbar = tqdm(
                total=steps_per_epoch,
                desc=f"Epoch {epoch + 1}/{args.epochs}",
            )
        else:
            pbar = None

        for input_ids, targets, loss_mask in train_loader:
            input_ids = input_ids.to(device)
            targets = targets.to(device)
            loss_mask = loss_mask.to(device)

            with autocast_ctx:
                result = model(input_ids)
                logits = result["logits"]

                # Masked cross-entropy
                B, T, V = logits.shape
                per_token_loss = F.cross_entropy(
                    logits.view(B * T, V), targets.view(B * T), reduction="none"
                )
                per_token_loss = per_token_loss.view(B, T)
                mask_sum = loss_mask.sum()
                if mask_sum > 0:
                    loss = (per_token_loss * loss_mask).sum() / mask_sum
                else:
                    loss = per_token_loss.mean()

                loss = loss / args.grad_accum

            loss.backward()

            if (global_step + 1) % args.grad_accum == 0:
                opt_step_count += 1
                lr = get_lr(
                    opt_step_count,
                    args.warmup_steps,
                    opt_steps,
                    args.lr,
                    args.lr * 0.1,
                )
                for pg in optimizer.param_groups:
                    pg["lr"] = lr

                # Clip gradients on trainable params only
                torch.nn.utils.clip_grad_norm_(trainable_params, args.grad_clip)
                optimizer.step()
                optimizer.zero_grad()

            global_step += 1
            actual_loss = loss.item() * args.grad_accum
            losses.append(actual_loss)

            if pbar is not None:
                ppl = math.exp(min(actual_loss, 20))
                pbar.update(1)
                pbar.set_postfix(
                    loss=f"{actual_loss:.3f}",
                    lr=f"{lr:.2e}",
                    ppl=f"{ppl:.1f}",
                )

            if not args.no_wandb and global_step % 10 == 0:
                import wandb

                ppl = math.exp(min(actual_loss, 20))
                wandb.log(
                    {
                        "train/loss": actual_loss,
                        "train/ppl": ppl,
                        "train/lr": lr,
                        "train/epoch": epoch + global_step / steps_per_epoch,
                    },
                    step=global_step,
                )

            # Periodic eval + checkpoint
            if global_step % args.save_interval == 0:
                eval_loss = evaluate(model, eval_loader, device, autocast_ctx)
                eval_ppl = math.exp(min(eval_loss, 20))
                print(
                    f"\n  Step {global_step}: eval_loss={eval_loss:.4f}, "
                    f"eval_ppl={eval_ppl:.1f}"
                )

                if not args.no_wandb:
                    import wandb

                    wandb.log(
                        {
                            "eval/loss": eval_loss,
                            "eval/ppl": eval_ppl,
                        },
                        step=global_step,
                    )

                raw_model = (
                    model._orig_mod if hasattr(model, "_orig_mod") else model
                )
                torch.save(
                    {
                        "model_state_dict": raw_model.state_dict(),
                        "optimizer_state_dict": optimizer.state_dict(),
                        "global_step": global_step,
                        "losses": losses[-100:],
                        "args": vars(args),
                    },
                    ckpt_dir / f"step_{global_step}.pt",
                )
                print(f"  Checkpoint saved at step {global_step}")

            # Smoke test early exit
            if args.smoke_test and global_step >= 50:
                break

        if pbar is not None:
            pbar.close()

        if args.smoke_test and global_step >= 50:
            break

    # Final eval
    eval_loss = evaluate(model, eval_loader, device, autocast_ctx)
    eval_ppl = math.exp(min(eval_loss, 20))

    # Final stats
    total_time = time.time() - start_time
    avg_loss = sum(losses[-100:]) / min(len(losses), 100) if losses else 0.0
    first_loss = sum(losses[:100]) / min(len(losses), 100) if losses else 0.0
    final_ppl = math.exp(min(avg_loss, 20))

    print(f"\n--- Fine-Tuning Complete ---")
    print(f"  Steps: {global_step} ({opt_step_count} optimizer steps)")
    print(f"  Time: {total_time:.0f}s ({total_time / 3600:.1f}h)")
    print(f"  First 100 avg loss: {first_loss:.3f}")
    print(f"  Last 100 avg loss: {avg_loss:.3f}")
    print(f"  Final train PPL: {final_ppl:.1f}")
    print(f"  Final eval loss: {eval_loss:.4f}, eval PPL: {eval_ppl:.1f}")
    print(f"  Loss decreased: {first_loss - avg_loss:.3f}")

    if avg_loss < first_loss:
        print("  PASS: Loss is decreasing.")
    else:
        print("  FAIL: Loss did not decrease!")

    # Save final checkpoint
    raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
    torch.save(
        {
            "model_state_dict": raw_model.state_dict(),
            "optimizer_state_dict": optimizer.state_dict(),
            "global_step": global_step,
            "losses": losses[-100:],
            "args": vars(args),
        },
        ckpt_dir / "last.pt",
    )
    print(f"  Checkpoint: {ckpt_dir}/last.pt")

    if not args.no_wandb:
        import wandb

        wandb.log(
            {"eval/final_loss": eval_loss, "eval/final_ppl": eval_ppl},
            step=global_step,
        )
        wandb.finish()


def main():
    parser = argparse.ArgumentParser(
        description="Fine-tune Felix-LM v3 spokes on mnemonic encoding data"
    )
    parser.add_argument(
        "--pretrained", type=str, required=True, help="Path to pretrained checkpoint"
    )
    parser.add_argument(
        "--train-data",
        type=str,
        default="training/data/finetune/train.jsonl",
        help="Path to train.jsonl",
    )
    parser.add_argument(
        "--eval-data",
        type=str,
        default="training/data/finetune/eval.jsonl",
        help="Path to eval.jsonl",
    )
    parser.add_argument("--lr", type=float, default=2e-4)
    parser.add_argument("--batch-size", type=int, default=2)
    parser.add_argument("--grad-accum", type=int, default=8)
    parser.add_argument("--seq-len", type=int, default=4096)
    parser.add_argument("--epochs", type=int, default=3)
    parser.add_argument(
        "--warmup-steps", type=int, default=0, help="0=auto (10%% of total)"
    )
    parser.add_argument("--weight-decay", type=float, default=0.01)
    parser.add_argument("--grad-clip", type=float, default=1.0)
    parser.add_argument("--save-interval", type=int, default=500)
    parser.add_argument("--compile", action="store_true")
    parser.add_argument(
        "--unfreeze-norm",
        action="store_true",
        help="Also unfreeze output_norm and embed_proj",
    )
    parser.add_argument(
        "--unfreeze-layers", type=int, default=0,
        help="Unfreeze last N transformer layers (0=spokes only)",
    )
    parser.add_argument(
        "--full-finetune",
        action="store_true",
        help="Unfreeze all parameters (full fine-tune)",
    )
    parser.add_argument(
        "--wandb-name", type=str, default="finetune_encoding",
    )
    parser.add_argument("--no-wandb", action="store_true")
    parser.add_argument(
        "--smoke-test",
        action="store_true",
        help="Run 50 steps with no wandb",
    )
    parser.add_argument(
        "--dtype", type=str, default="bf16", choices=["bf16", "fp32"]
    )
    parser.add_argument("--device", type=str, default="cuda")
    args = parser.parse_args()

    if args.smoke_test:
        args.no_wandb = True
        print("=== SMOKE TEST MODE (50 steps, no wandb) ===\n")

    config = make_v3_mnemonic_100m_config()
    train(config, args)


if __name__ == "__main__":
    main()
