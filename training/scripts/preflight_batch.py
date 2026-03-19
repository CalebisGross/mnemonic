#!/usr/bin/env python3
"""Find max batch size for mnemonic-LM configs via binary search.

Adapted from felixlm/scripts/debug_nan.py. Runs forward+backward passes
in-process with try/except torch.OutOfMemoryError — safe, no OOM killer risk.

Usage:
    source ~/Projects/felixlm/.venv/bin/activate
    python training/scripts/preflight_batch.py
    python training/scripts/preflight_batch.py --config v3_mnemonic_100m --max-try 32
    python training/scripts/preflight_batch.py --steps 10 --safety-margin 0.75
"""

import argparse
import sys
from pathlib import Path

import torch

# Add mnemonic training scripts to path for CONFIGS
TRAINING_DIR = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(TRAINING_DIR / "scripts"))

from train_mnemonic_lm import CONFIGS


def find_max_batch(config_name, max_try=32, steps=5, compile_model=False):
    """Binary search for max batch size that fits in memory.

    Runs `steps` forward+backward passes per candidate to catch delayed OOMs.
    """
    config = CONFIGS[config_name]()
    lo, hi, best = 1, max_try, 0

    while lo <= hi:
        mid = (lo + hi) // 2
        model = None
        optimizer = None
        try:
            torch.cuda.empty_cache()
            from felix_lm.v3.model import FelixLMv3

            model = FelixLMv3(config).cuda()
            if compile_model:
                model = torch.compile(model)
            optimizer = torch.optim.AdamW(model.parameters(), lr=6e-4)

            for step in range(steps):
                x = torch.randint(0, config.vocab_size, (mid, 2048)).cuda()
                with torch.autocast("cuda", dtype=torch.bfloat16):
                    result = model(x, x)
                loss_val = result["loss"].item()
                if step == 0 and loss_val != loss_val:  # NaN check
                    raise ValueError("NaN loss on step 0!")
                result["loss"].backward()
                optimizer.step()
                optimizer.zero_grad()
                del x, result

            del model, optimizer
            torch.cuda.empty_cache()
            model = None
            optimizer = None
            best = mid
            print(f"  {config_name} bs={mid}: OK ({steps} steps)")
            lo = mid + 1
        except torch.OutOfMemoryError:
            if model is not None:
                del model
            if optimizer is not None:
                del optimizer
            torch.cuda.empty_cache()
            print(f"  {config_name} bs={mid}: OOM")
            hi = mid - 1
        except Exception as e:
            if model is not None:
                del model
            if optimizer is not None:
                del optimizer
            torch.cuda.empty_cache()
            print(f"  {config_name} bs={mid}: ERROR: {e}")
            hi = mid - 1

    return best


def main():
    parser = argparse.ArgumentParser(
        description="Find max batch size for mnemonic-LM configs"
    )
    parser.add_argument(
        "--config",
        type=str,
        default=None,
        choices=list(CONFIGS.keys()),
        help="Test a single config (default: all)",
    )
    parser.add_argument(
        "--max-try", type=int, default=32, help="Upper bound for binary search"
    )
    parser.add_argument(
        "--steps", type=int, default=5, help="Forward+backward steps per candidate"
    )
    parser.add_argument(
        "--safety-margin",
        type=float,
        default=0.75,
        help="Multiply max by this for safe batch size (default: 0.75)",
    )
    parser.add_argument("--compile", action="store_true", help="Test with torch.compile")
    args = parser.parse_args()

    configs = [args.config] if args.config else list(CONFIGS.keys())

    print(f"GPU: {torch.cuda.get_device_name(0)}")
    props = torch.cuda.get_device_properties(0)
    vram = getattr(props, "total_memory", getattr(props, "total_mem", 0))
    print(f"VRAM: {vram / 1e9:.0f} GB")
    print(f"Testing {len(configs)} configs, {args.steps} steps each, max_try={args.max_try}")
    print()

    results = {}
    for name in configs:
        print(f"=== {name} ===")
        max_bs = find_max_batch(
            name, max_try=args.max_try, steps=args.steps, compile_model=args.compile
        )
        safe_bs = max(2, int(max_bs * args.safety_margin) & ~1)  # round down to even
        results[name] = (max_bs, safe_bs)
        print(f"  -> Max: {max_bs}, Safe ({args.safety_margin:.0%}): {safe_bs}\n")

    print("=" * 50)
    print("SUMMARY")
    print("=" * 50)
    for name, (max_bs, safe_bs) in results.items():
        print(f"  {name}: max={max_bs}, safe={safe_bs}")
        print(f"    Recommended: --batch-size {safe_bs} --grad-accum {max(1, 256 // safe_bs)}")


if __name__ == "__main__":
    main()
