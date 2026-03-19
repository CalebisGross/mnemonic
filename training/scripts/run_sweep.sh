#!/usr/bin/env bash
# Run remaining HP sweep configs for EXP-2 (LR + Weight Decay).
#
# Each run: 4000 micro-steps (1000 optimizer steps) at batch 10 / accum 4.
# Results are appended to sweep_results.tsv after each run completes.
#
# Usage:
#   ./training/scripts/run_sweep.sh              # run all remaining
#   ./training/scripts/run_sweep.sh lr2e3_wd01   # run a single config

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRAINING_DIR="$(dirname "$SCRIPT_DIR")"
TRAIN_SCRIPT="$SCRIPT_DIR/train_mnemonic_lm.py"
TSV="$TRAINING_DIR/sweep_results.tsv"

# Shared args
CONFIG="v3_mnemonic_100m"
BATCH=10
ACCUM=4
STEPS=4000
BETA2=0.95
DEVICE="cuda"
WANDB_GROUP="hp_sweep_v3_100m"

# Sweep grid: name -> "lr wd"
declare -A SWEEP_GRID=(
    ["lr2e3_wd01"]="2e-3 0.1"
    ["lr6e4_wd005"]="6e-4 0.05"
    ["lr1e3_wd005"]="1e-3 0.05"
)

# Ordered run list (bash associative arrays don't preserve order)
SWEEP_ORDER=("lr2e3_wd01" "lr6e4_wd005" "lr1e3_wd005")

run_one() {
    local name="$1"
    local lr wd
    read -r lr wd <<< "${SWEEP_GRID[$name]}"
    local run_name="sweep_${name}"

    # Skip if already in TSV
    if grep -q "^${run_name}	" "$TSV" 2>/dev/null; then
        echo "=== SKIP: $run_name (already in $TSV) ==="
        return 0
    fi

    echo ""
    echo "========================================"
    echo "  SWEEP: $run_name"
    echo "  LR=$lr  WD=$wd  batch=$BATCH  accum=$ACCUM  steps=$STEPS"
    echo "========================================"
    echo ""

    local start_time
    start_time=$(date +%s)

    python "$TRAIN_SCRIPT" \
        --config "$CONFIG" \
        --device "$DEVICE" \
        --batch-size "$BATCH" \
        --grad-accum "$ACCUM" \
        --lr "$lr" \
        --weight-decay "$wd" \
        --beta2 "$BETA2" \
        --max-steps "$STEPS" \
        --compile \
        --wandb-name "$run_name" \
        2>&1 | tee "/tmp/${run_name}.log"

    local end_time elapsed
    end_time=$(date +%s)
    elapsed=$((end_time - start_time))

    # Parse final loss and PPL from training output
    local final_loss final_ppl
    final_loss=$(grep "Last 100 avg loss:" "/tmp/${run_name}.log" | awk '{print $NF}')
    final_ppl=$(grep "Final PPL:" "/tmp/${run_name}.log" | awk '{print $NF}')

    if [[ -z "$final_loss" || -z "$final_ppl" ]]; then
        echo "ERROR: Could not parse results from $run_name — check /tmp/${run_name}.log"
        return 1
    fi

    # Append to TSV
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$run_name" "$final_loss" "$final_ppl" "$lr" "$wd" "$BETA2" "auto" "$BATCH" "$ACCUM" "$STEPS" "$elapsed" \
        >> "$TSV"

    echo ""
    echo "=== DONE: $run_name — loss=$final_loss ppl=$final_ppl time=${elapsed}s ==="
    echo "=== Logged to $TSV ==="
    echo ""
}

# Main
if [[ $# -gt 0 ]]; then
    # Run specific config(s)
    for name in "$@"; do
        if [[ -z "${SWEEP_GRID[$name]+x}" ]]; then
            echo "ERROR: Unknown sweep config '$name'"
            echo "Available: ${!SWEEP_GRID[*]}"
            exit 1
        fi
        run_one "$name"
    done
else
    # Run all remaining
    echo "=== HP Sweep: ${#SWEEP_ORDER[@]} configs remaining ==="
    echo "=== Estimated time: ~7h (3 x ~2.3h each) ==="
    echo ""
    for name in "${SWEEP_ORDER[@]}"; do
        run_one "$name"
    done
    echo ""
    echo "=== All sweep runs complete ==="
    echo "=== Results in $TSV ==="
    cat "$TSV"
fi
