#!/usr/bin/env python3
"""Train Felix-LM v3 on mnemonic's curated data mix.

Bridge script that connects the mnemonic MixedPretrainDataset to
Felix-LM's training infrastructure. Runs from the mnemonic repo
but imports Felix-LM's model and training utilities.

Usage:
    python training/scripts/train_mnemonic_lm.py --config v3_mnemonic_500m --device cuda
    python training/scripts/train_mnemonic_lm.py --config v3_mnemonic_500m --device cuda --smoke-test
    python training/scripts/train_mnemonic_lm.py --config v3_mnemonic_100m --device cuda --resume last.pt

Requires:
    - Felix-LM installed/importable: pip install -e ~/Projects/felixlm
    - Tokenized shards in training/data/pretrain/tokenized/
"""

import argparse
import gc
import math
import sys
import time
from pathlib import Path

import numpy as np
import torch
from torch.utils.data import DataLoader, IterableDataset

# Add mnemonic training scripts to path
TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))

from dataset import MixedPretrainDataset


# --- Mnemonic-specific configs ---

def make_v3_mnemonic_500m_config():
    """Felix-LM v3 at ~500M params with mnemonic's 32K vocab."""
    from felix_lm.v3.config import FelixV3Config

    return FelixV3Config(
        vocab_size=32768,  # Our custom BPE tokenizer
        d_embed=1024,
        num_layers=24,
        num_heads=16,
        num_spokes=4,
        spoke_rank=64,
        gate_schedule="uniform",
        embed_proj=True,
        dropout=0.1,
        gradient_checkpointing=True,
    )


def make_v3_mnemonic_100m_config():
    """Felix-LM v3 at ~100M params with mnemonic's 32K vocab. For testing."""
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


CONFIGS = {
    "v3_mnemonic_500m": make_v3_mnemonic_500m_config,
    "v3_mnemonic_100m": make_v3_mnemonic_100m_config,
}


# --- PyTorch Dataset Wrapper ---

class MnemonicDataset(IterableDataset):
    """PyTorch IterableDataset wrapper around MixedPretrainDataset."""

    def __init__(self, config_path: str, seq_len: int = 2048, tokenized_dir: str | None = None):
        self.config_path = config_path
        self.seq_len = seq_len
        self.tokenized_dir = tokenized_dir
        # Don't initialize the dataset here — do it in __iter__
        # so each DataLoader worker gets its own instance
        self._ds = None

    def __iter__(self):
        if self._ds is None:
            self._ds = MixedPretrainDataset(
                self.config_path,
                seq_len=self.seq_len,
                tokenized_dir=self.tokenized_dir,
            )

        for input_ids, targets in self._ds:
            yield (
                torch.from_numpy(input_ids),
                torch.from_numpy(targets),
            )


# --- LR Schedule (same as Felix-LM) ---

def get_lr(step: int, warmup_steps: int, max_steps: int, max_lr: float, min_lr: float) -> float:
    """Cosine learning rate schedule with linear warmup."""
    if step < warmup_steps:
        return max_lr * step / warmup_steps
    if step >= max_steps:
        return min_lr
    progress = (step - warmup_steps) / (max_steps - warmup_steps)
    return min_lr + 0.5 * (max_lr - min_lr) * (1 + math.cos(math.pi * progress))


# --- Training ---

def train(config, args):
    from felix_lm.v3.model import FelixLMv3
    from felix_lm.utils import count_parameters

    device = torch.device(args.device)

    # Cap VRAM to 90% — OOM becomes a catchable PyTorch exception
    # instead of triggering the Linux OOM killer and crashing the system
    if device.type == "cuda":
        torch.cuda.set_per_process_memory_fraction(0.9)

    model = FelixLMv3(config).to(device)
    if args.compile:
        print("Compiling model with torch.compile...")
        model = torch.compile(model)

    n_params = count_parameters(model)
    spoke_info = (
        f"{config.num_spokes} spokes r={config.spoke_rank}"
        if config.gate_schedule != "none"
        else "no spokes"
    )
    print(
        f"\nModel: Felix-LM v3 ({config.num_layers}L, d={config.d_embed}, "
        f"vocab={config.vocab_size}, {spoke_info}), {n_params:,} params"
    )

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
    config_path = str(TRAINING_DIR / "configs" / "pretrain_mix.yaml")
    tokenized_dir = args.tokenized_dir or str(TRAINING_DIR / "data" / "pretrain" / "tokenized")
    train_ds = MnemonicDataset(config_path, seq_len=args.seq_len, tokenized_dir=tokenized_dir)
    train_loader = DataLoader(train_ds, batch_size=args.batch_size, num_workers=0, pin_memory=True)

    # Optimizer (with spoke-specific LR)
    spoke_lr_mult = args.spoke_lr_mult
    if spoke_lr_mult != 1.0 and config.gate_schedule != "none":
        spoke_params = []
        other_params = []
        raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
        spoke_ids = set()
        if raw_model.spokes is not None:
            for name, param in raw_model.spokes.named_parameters():
                spoke_ids.add(id(param))
                spoke_params.append(param)
        for param in model.parameters():
            if id(param) not in spoke_ids:
                other_params.append(param)
        spoke_lr = args.lr * spoke_lr_mult
        param_groups = [
            {"params": other_params, "lr": args.lr, "base_lr": args.lr},
            {"params": spoke_params, "lr": spoke_lr, "base_lr": spoke_lr},
        ]
        print(f"  Spoke LR: {spoke_lr:.4f} ({spoke_lr_mult}x backbone)")
    else:
        param_groups = [
            {"params": list(model.parameters()), "lr": args.lr, "base_lr": args.lr},
        ]
    optimizer = torch.optim.AdamW(
        param_groups,
        lr=args.lr,
        weight_decay=args.weight_decay,
        betas=(0.9, args.beta2),
    )

    tokens_per_step = args.batch_size * args.seq_len * args.grad_accum
    max_steps = args.max_steps
    opt_steps = max_steps // args.grad_accum
    print(f"\nTraining: {max_steps} steps ({opt_steps} optimizer steps)")
    print(f"  grad_accum={args.grad_accum}, effective batch={args.batch_size * args.grad_accum}")
    print(f"  Tokens/step: {tokens_per_step:,}")
    print(f"  Total tokens: ~{max_steps * args.batch_size * args.seq_len:,}")

    # Auto-scale warmup
    if args.warmup_steps == 0:
        args.warmup_steps = max(1, opt_steps // 10)
    print(f"  Warmup: {args.warmup_steps} optimizer steps")

    # Training loop
    ckpt_dir = Path(args.ckpt_dir) if args.ckpt_dir else Path(f"checkpoints/{args.config}")
    ckpt_dir.mkdir(parents=True, exist_ok=True)
    global_step = 0
    lr = args.lr
    losses = []

    # Resume from checkpoint
    if args.resume:
        resume_path = ckpt_dir / args.resume if not Path(args.resume).is_absolute() else Path(args.resume)
        if resume_path.exists():
            print(f"\nResuming from {resume_path}...")
            ckpt = torch.load(resume_path, map_location=device, weights_only=False)
            if "model_state_dict" in ckpt:
                # Full checkpoint (model + optimizer + step)
                raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
                raw_model.load_state_dict(ckpt["model_state_dict"])
                optimizer.load_state_dict(ckpt["optimizer_state_dict"])
                global_step = ckpt["global_step"]
                losses = ckpt.get("recent_losses", [])
                if "rng_state" in ckpt:
                    torch.set_rng_state(ckpt["rng_state"])
                if "cuda_rng_state" in ckpt and device.type == "cuda":
                    torch.cuda.set_rng_state(ckpt["cuda_rng_state"])
                print(f"  Resumed at global_step={global_step} (opt_step={global_step // args.grad_accum})")
            else:
                # Legacy checkpoint (model weights only) — start optimizer fresh
                raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
                raw_model.load_state_dict(ckpt)
                print("  Loaded model weights only (legacy checkpoint, optimizer reset)")
        else:
            print(f"WARNING: Resume path {resume_path} not found, training from scratch")

    # wandb (after resume so we know the step count)
    if not args.no_wandb:
        import wandb
        wandb_kwargs = {
            "project": "mnemonic-lm",
            "name": args.run_name or args.config,
            "config": {
                "model_params": n_params,
                "config": args.config,
                "vocab_size": config.vocab_size,
                "lr": args.lr,
                "weight_decay": args.weight_decay,
                "beta2": args.beta2,
                "batch_size": args.batch_size,
                "grad_accum": args.grad_accum,
                "seq_len": args.seq_len,
                "max_steps": max_steps,
                "spoke_lr_mult": args.spoke_lr_mult,
                "warmup_steps": args.warmup_steps,
                "data": "mnemonic-curated-6.5B",
            },
        }
        if args.wandb_group:
            wandb_kwargs["group"] = args.wandb_group
        if args.wandb_tags:
            wandb_kwargs["tags"] = [t.strip() for t in args.wandb_tags.split(",")]
        if args.resume and global_step > 0:
            wandb_kwargs["resume"] = "must"
            print(f"  wandb: resuming run (step {global_step})")
        wandb.init(**wandb_kwargs)

    model.train()
    optimizer.zero_grad()

    # Fast-forward dataloader if resuming (skip already-seen batches)
    if global_step > 0:
        print(f"  Fast-forwarding dataloader past {global_step} batches...")
        skip_iter = iter(train_loader)
        for i in range(global_step):
            next(skip_iter, None)
            if (i + 1) % 1000 == 0:
                print(f"    Skipped {i + 1}/{global_step} batches")
        print(f"  Fast-forward complete, resuming training at step {global_step}")
        train_iter = skip_iter
    else:
        train_iter = iter(train_loader)

    start_time = time.time()

    try:
        from tqdm import tqdm
        pbar = tqdm(total=max_steps, initial=global_step, desc="Training")
    except ImportError:
        pbar = None

    for input_ids, targets in train_iter:
        input_ids = input_ids.to(device)
        targets = targets.to(device)

        with autocast_ctx:
            result = model(input_ids, targets)
            loss = result["loss"] / args.grad_accum

        loss.backward()

        if (global_step + 1) % args.grad_accum == 0:
            opt_step = global_step // args.grad_accum
            lr = get_lr(opt_step, args.warmup_steps, opt_steps, args.lr, args.lr * 0.1)
            schedule_mult = lr / args.lr if args.lr > 0 else 1.0
            for pg in optimizer.param_groups:
                base = pg.get("base_lr", args.lr)
                pg["lr"] = base * schedule_mult
            torch.nn.utils.clip_grad_norm_(model.parameters(), args.grad_clip)
            optimizer.step()
            optimizer.zero_grad()

            if opt_step == 1:
                gc.collect()
                gc.freeze()
                gc.disable()

        global_step += 1
        actual_loss = loss.item() * args.grad_accum
        losses.append(actual_loss)

        if pbar:
            ppl = math.exp(min(actual_loss, 20))
            pbar.update(1)
            pbar.set_postfix(loss=f"{actual_loss:.3f}", lr=f"{lr:.2e}", ppl=f"{ppl:.1f}")

        if not args.no_wandb and global_step % 10 == 0:
            import wandb
            ppl = math.exp(min(actual_loss, 20))
            log_dict = {"train/loss": actual_loss, "train/ppl": ppl, "train/lr": lr}
            if "gate_values" in result:
                for i, gv in enumerate(result["gate_values"]):
                    log_dict[f"v3/gate_layer_{i}"] = gv.item()
            wandb.log(log_dict, step=global_step)

        # Periodic checkpoint (full state for resume)
        if global_step % args.save_interval == 0:
            raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
            ckpt_data = {
                "model_state_dict": raw_model.state_dict(),
                "optimizer_state_dict": optimizer.state_dict(),
                "global_step": global_step,
                "recent_losses": losses[-1000:],
                "rng_state": torch.get_rng_state(),
                "args": vars(args),
            }
            if device.type == "cuda":
                ckpt_data["cuda_rng_state"] = torch.cuda.get_rng_state()
            torch.save(ckpt_data, ckpt_dir / f"step_{global_step}.pt")
            # Also save as last.pt for easy resume
            torch.save(ckpt_data, ckpt_dir / "last.pt")
            print(f"\n  Checkpoint saved at step {global_step}")

        if global_step % 5000 == 0 and global_step > 0:
            gc.enable()
            gc.collect()
            gc.freeze()
            gc.disable()

        if global_step >= max_steps:
            break

    if pbar:
        pbar.close()

    gc.enable()

    # Final stats
    total_time = time.time() - start_time
    avg_loss = sum(losses[-100:]) / min(len(losses), 100)
    first_loss = sum(losses[:100]) / min(len(losses), 100)
    final_ppl = math.exp(min(avg_loss, 20))

    print(f"\n--- Training Complete ---")
    print(f"  Steps: {global_step}")
    print(f"  Time: {total_time:.0f}s ({total_time / 3600:.1f}h)")
    print(f"  First 100 avg loss: {first_loss:.3f}")
    print(f"  Last 100 avg loss: {avg_loss:.3f}")
    print(f"  Final PPL: {final_ppl:.1f}")
    print(f"  Loss decreased: {first_loss - avg_loss:.3f}")

    if avg_loss < first_loss:
        print("  PASS: Loss is decreasing.")
    else:
        print("  FAIL: Loss did not decrease!")

    raw_model = model._orig_mod if hasattr(model, "_orig_mod") else model
    ckpt_data = {
        "model_state_dict": raw_model.state_dict(),
        "optimizer_state_dict": optimizer.state_dict(),
        "global_step": global_step,
        "recent_losses": losses[-1000:],
        "args": vars(args),
    }
    torch.save(ckpt_data, ckpt_dir / "last.pt")
    print(f"  Checkpoint: {ckpt_dir}/last.pt")

    if not args.no_wandb:
        import wandb
        wandb.finish()


def main():
    parser = argparse.ArgumentParser(description="Train Felix-LM v3 on mnemonic data")
    parser.add_argument("--config", type=str, default="v3_mnemonic_100m", choices=list(CONFIGS.keys()))
    parser.add_argument("--device", type=str, default="cuda")
    parser.add_argument("--batch-size", type=int, default=8)
    parser.add_argument("--grad-accum", type=int, default=4)
    parser.add_argument("--seq-len", type=int, default=2048)
    parser.add_argument("--lr", type=float, default=6e-4)
    parser.add_argument("--weight-decay", type=float, default=0.1)
    parser.add_argument("--warmup-steps", type=int, default=0, help="0=auto (10%% of total)")
    parser.add_argument("--grad-clip", type=float, default=1.0)
    parser.add_argument("--max-steps", type=int, default=100000)
    parser.add_argument("--save-interval", type=int, default=5000)
    parser.add_argument("--no-wandb", action="store_true")
    parser.add_argument("--dtype", type=str, default="bf16", choices=["bf16", "fp32"])
    parser.add_argument("--beta2", type=float, default=0.95)
    parser.add_argument("--compile", action="store_true")
    parser.add_argument("--spoke-lr-mult", type=float, default=2.0, help="Spoke LR multiplier")
    parser.add_argument("--tokenized-dir", type=str, default=None)
    parser.add_argument("--ckpt-dir", type=str, default=None, help="Checkpoint directory (default: checkpoints/<config>)")
    parser.add_argument("--resume", type=str, default=None,
                        help="Resume from checkpoint (filename in ckpt dir, or absolute path). Use 'last.pt' to resume latest.")
    parser.add_argument("--run-name", type=str, default=None, help="wandb run name (default: config name)")
    parser.add_argument("--wandb-group", type=str, default=None, help="wandb group for sweep comparison")
    parser.add_argument("--wandb-tags", type=str, default=None, help="Comma-separated wandb tags")
    parser.add_argument("--smoke-test", action="store_true", help="Run 1000 steps to verify pipeline")
    args = parser.parse_args()

    if args.smoke_test:
        args.max_steps = 1000
        args.no_wandb = True
        args.save_interval = 500
        print("=== SMOKE TEST MODE (1000 steps, no wandb) ===\n")

    config = CONFIGS[args.config]()
    train(config, args)


if __name__ == "__main__":
    main()
