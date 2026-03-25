#!/usr/bin/env bash
# Autoresearch sweep for fine-tuning hyperparameters.
#
# Phase 1: Unfreeze strategy (1 epoch each, ~30 min per probe)
#   - spokes only
#   - spokes + norm
#   - spokes + last 4 layers + norm
#   - full fine-tune
#
# Phase 2: LR sweep on the winning strategy (1 epoch each)
#
# Results are logged to training/finetune_sweep_results.tsv
#
# Usage:
#   ./training/scripts/sweep_finetune.sh                    # run all
#   ./training/scripts/sweep_finetune.sh --phase 1          # unfreeze probes only
#   ./training/scripts/sweep_finetune.sh --phase 2          # LR sweep only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRAINING_DIR="$(dirname "$SCRIPT_DIR")"
FT_SCRIPT="$SCRIPT_DIR/finetune_mnemonic.py"
EVAL_SCRIPT="$SCRIPT_DIR/eval_encoding.py"
TSV="$TRAINING_DIR/finetune_sweep_results.tsv"
PRETRAINED="checkpoints/v3_mnemonic_100m/step_100000.pt"

# Create TSV header if it doesn't exist
if [[ ! -f "$TSV" ]]; then
    printf 'run_name\tstrategy\tlr\tepochs\ttrain_loss\ttrain_ppl\teval_loss\teval_ppl\ttrainable_params\ttime_s\n' > "$TSV"
fi

run_probe() {
    local name="$1"
    local strategy="$2"  # spokes, spokes_norm, spokes_last4, full
    local lr="$3"
    local epochs="$4"

    # Skip if already in TSV
    if grep -q "^${name}	" "$TSV" 2>/dev/null; then
        echo "=== SKIP: $name (already in $TSV) ==="
        return 0
    fi

    echo ""
    echo "========================================"
    echo "  PROBE: $name"
    echo "  Strategy: $strategy  LR: $lr  Epochs: $epochs"
    echo "========================================"
    echo ""

    # Build args based on strategy
    local extra_args=""
    case "$strategy" in
        spokes)
            extra_args=""
            ;;
        spokes_norm)
            extra_args="--unfreeze-norm"
            ;;
        spokes_last4)
            extra_args="--unfreeze-norm --unfreeze-layers 4"
            ;;
        spokes_last8)
            extra_args="--unfreeze-norm --unfreeze-layers 8"
            ;;
        full)
            extra_args="--full-finetune"
            ;;
        *)
            echo "ERROR: Unknown strategy '$strategy'"
            return 1
            ;;
    esac

    local ckpt_dir="checkpoints/ft_sweep_${name}"
    local start_time
    start_time=$(date +%s)

    python "$FT_SCRIPT" \
        --pretrained "$PRETRAINED" \
        --lr "$lr" \
        --epochs "$epochs" \
        --no-wandb \
        $extra_args \
        2>&1 | tee "/tmp/ft_sweep_${name}.log"

    local end_time elapsed
    end_time=$(date +%s)
    elapsed=$((end_time - start_time))

    # Parse results from log
    local train_loss train_ppl eval_loss eval_ppl trainable
    train_loss=$(grep "Last 100 avg loss:" "/tmp/ft_sweep_${name}.log" | awk '{print $NF}')
    train_ppl=$(grep "Final train PPL:" "/tmp/ft_sweep_${name}.log" | awk '{print $NF}')
    eval_loss=$(grep "Final eval loss:" "/tmp/ft_sweep_${name}.log" | grep -oP 'eval_loss=\K[0-9.]+' || grep "Final eval loss:" "/tmp/ft_sweep_${name}.log" | awk -F',' '{print $1}' | awk '{print $NF}')
    eval_ppl=$(grep "eval PPL:" "/tmp/ft_sweep_${name}.log" | grep -oP 'eval_ppl=\K[0-9.]+' || grep "eval PPL:" "/tmp/ft_sweep_${name}.log" | awk '{print $NF}')
    trainable=$(grep "Trainable:" "/tmp/ft_sweep_${name}.log" | grep -oP 'Trainable: \K[0-9,]+' | tr -d ',')

    if [[ -z "$eval_loss" ]]; then
        echo "WARNING: Could not parse eval_loss from log"
        eval_loss="N/A"
        eval_ppl="N/A"
    fi

    # Append to TSV
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$name" "$strategy" "$lr" "$epochs" \
        "${train_loss:-N/A}" "${train_ppl:-N/A}" \
        "${eval_loss:-N/A}" "${eval_ppl:-N/A}" \
        "${trainable:-N/A}" "$elapsed" \
        >> "$TSV"

    echo ""
    echo "=== DONE: $name — eval_loss=$eval_loss eval_ppl=$eval_ppl time=${elapsed}s ==="
    echo ""
}

# Parse args
PHASE="${1:---all}"
if [[ "$PHASE" == "--phase" ]]; then
    PHASE="${2:-1}"
fi

# ============================================================
# Phase 1: Unfreeze strategy sweep (fixed LR=5e-4, 1 epoch)
# ============================================================
if [[ "$PHASE" == "--all" || "$PHASE" == "1" ]]; then
    echo "=== Phase 1: Unfreeze Strategy Sweep ==="
    echo "=== Fixed: LR=5e-4, 1 epoch ==="
    echo ""

    run_probe "p1_spokes"       "spokes"       "5e-4" "1"
    run_probe "p1_spokes_norm"  "spokes_norm"  "5e-4" "1"
    run_probe "p1_last4"        "spokes_last4" "5e-4" "1"
    run_probe "p1_full"         "full"         "5e-4" "1"

    echo ""
    echo "=== Phase 1 Complete ==="
    echo "=== Results: ==="
    cat "$TSV"
    echo ""
    echo "Pick the strategy with the lowest eval_loss for Phase 2."
fi

# ============================================================
# Phase 2: LR sweep on best strategy
# (Edit the strategy below based on Phase 1 results)
# ============================================================
if [[ "$PHASE" == "--all" || "$PHASE" == "2" ]]; then
    # Default to full — edit after Phase 1 if needed
    BEST_STRATEGY="${BEST_STRATEGY:-full}"

    echo "=== Phase 2: LR Sweep (strategy=$BEST_STRATEGY) ==="
    echo ""

    run_probe "p2_lr1e4"  "$BEST_STRATEGY" "1e-4" "3"
    run_probe "p2_lr2e4"  "$BEST_STRATEGY" "2e-4" "3"
    run_probe "p2_lr5e4"  "$BEST_STRATEGY" "5e-4" "3"
    run_probe "p2_lr1e3"  "$BEST_STRATEGY" "1e-3" "3"

    echo ""
    echo "=== Phase 2 Complete ==="
    cat "$TSV"
fi

echo ""
echo "=== All sweeps done. Results in $TSV ==="
