#!/usr/bin/env bash
# Binary search for optimal learning rate.
#
# Uses short probe runs (1000 micro-steps, ~35min each) to bracket the
# optimum, then runs one full confirmation (4000 steps) at the best LR.
#
# Usage:
#   ./training/scripts/bisect_lr.sh                    # auto: bracket [2e-3, 2e-2], 3 rounds
#   ./training/scripts/bisect_lr.sh 2e-3 2e-2 3        # explicit: lo hi rounds
#   ./training/scripts/bisect_lr.sh --confirm 4.5e-3   # skip bisection, run full 4000-step confirmation

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRAINING_DIR="$(dirname "$SCRIPT_DIR")"
TRAIN_SCRIPT="$SCRIPT_DIR/train_mnemonic_lm.py"
TSV="$TRAINING_DIR/sweep_results.tsv"
PROBE_TSV="$TRAINING_DIR/probe_results.tsv"

# Shared training args
CONFIG="v3_mnemonic_100m"
BATCH=10
ACCUM=4
BETA2=0.95
WD=0.1
DEVICE="cuda"

PROBE_STEPS=1000
FULL_STEPS=4000

# --- helpers ---

run_probe() {
    local lr="$1"
    local steps="${2:-$PROBE_STEPS}"
    local tag="${3:-probe}"
    local run_name="${tag}_lr${lr}"

    echo ""
    echo "========================================"
    echo "  ${tag^^}: LR=$lr  steps=$steps"
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
        --weight-decay "$WD" \
        --beta2 "$BETA2" \
        --max-steps "$steps" \
        --compile \
        --no-wandb \
        --save-interval 99999 \
        2>&1 | tee "/tmp/${run_name}.log"

    local end_time elapsed
    end_time=$(date +%s)
    elapsed=$((end_time - start_time))

    local final_loss final_ppl
    final_loss=$(grep "Last 100 avg loss:" "/tmp/${run_name}.log" | awk '{print $NF}')
    final_ppl=$(grep "Final PPL:" "/tmp/${run_name}.log" | awk '{print $NF}')

    if [[ -z "$final_loss" || -z "$final_ppl" ]]; then
        echo "ERROR: Could not parse results — check /tmp/${run_name}.log"
        return 1
    fi

    # Init probe TSV if needed
    if [[ ! -f "$PROBE_TSV" ]]; then
        printf 'run_name\tfinal_loss\tfinal_ppl\tlr\twd\tsteps\ttime_s\n' > "$PROBE_TSV"
    fi

    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$run_name" "$final_loss" "$final_ppl" "$lr" "$WD" "$steps" "$elapsed" \
        >> "$PROBE_TSV"

    echo ""
    echo "=== ${tag^^} DONE: LR=$lr — loss=$final_loss ppl=$final_ppl time=${elapsed}s ==="
    echo ""

    # Export for caller
    LAST_LOSS="$final_loss"
    LAST_PPL="$final_ppl"
}

# Compare two floats: returns 0 if $1 < $2
float_lt() {
    python3 -c "import sys; sys.exit(0 if float('$1') < float('$2') else 1)"
}

# Geometric midpoint of two LRs (midpoint in log-space)
midpoint_lr() {
    python3 -c "
import math
lo, hi = float('$1'), float('$2')
mid = math.exp((math.log(lo) + math.log(hi)) / 2)
# Round to 1 significant figure for clean naming
from math import log10, floor
digits = -floor(log10(abs(mid))) + 1
mid = round(mid, digits)
print(f'{mid:.1e}')
"
}

# --- confirm mode ---

if [[ "${1:-}" == "--confirm" ]]; then
    lr="${2:?Usage: bisect_lr.sh --confirm <lr>}"
    echo "=== CONFIRMATION RUN: LR=$lr at $FULL_STEPS steps ==="
    run_name="sweep_bisect_lr${lr}_wd01"

    if grep -q "^${run_name}	" "$TSV" 2>/dev/null; then
        echo "Already in $TSV — skipping"
        exit 0
    fi

    local_start=$(date +%s)

    python "$TRAIN_SCRIPT" \
        --config "$CONFIG" \
        --device "$DEVICE" \
        --batch-size "$BATCH" \
        --grad-accum "$ACCUM" \
        --lr "$lr" \
        --weight-decay "$WD" \
        --beta2 "$BETA2" \
        --max-steps "$FULL_STEPS" \
        --compile \
        --wandb-name "$run_name" \
        2>&1 | tee "/tmp/${run_name}.log"

    local_end=$(date +%s)
    elapsed=$((local_end - local_start))

    final_loss=$(grep "Last 100 avg loss:" "/tmp/${run_name}.log" | awk '{print $NF}')
    final_ppl=$(grep "Final PPL:" "/tmp/${run_name}.log" | awk '{print $NF}')

    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$run_name" "$final_loss" "$final_ppl" "$lr" "$WD" "$BETA2" "auto" "$BATCH" "$ACCUM" "$FULL_STEPS" "$elapsed" \
        >> "$TSV"

    echo ""
    echo "=== CONFIRMED: LR=$lr — loss=$final_loss ppl=$final_ppl ==="
    echo "=== Logged to $TSV ==="
    exit 0
fi

# --- bisection mode ---

LO_LR="${1:-2e-3}"
HI_LR="${2:-2e-2}"
ROUNDS="${3:-3}"

echo "=== LR BISECTION SEARCH ==="
echo "  Bracket: [$LO_LR, $HI_LR]"
echo "  Rounds: $ROUNDS (probe @ $PROBE_STEPS steps each, ~35 min/round)"
echo "  Confirmation: 1 full run @ $FULL_STEPS steps (~2.3h)"
echo "  Total estimated time: ~$(( (ROUNDS * 35 + 140) ))min"
echo ""

# We know 2e-3 from the sweep: loss 4.250
LO_LOSS="4.250"
HI_LOSS=""

# Step 1: probe the upper bound
echo "--- Round 0: probe upper bound LR=$HI_LR ---"
run_probe "$HI_LR"
HI_LOSS="$LAST_LOSS"

if float_lt "$HI_LOSS" "$LO_LOSS"; then
    echo "Upper bound ($HI_LR) beats lower ($LO_LR): $HI_LOSS < $LO_LOSS"
    echo "Optimum is at or above $HI_LR — consider widening the bracket."
    echo "Using $HI_LR as best candidate for confirmation."
    BEST_LR="$HI_LR"
    BEST_LOSS="$HI_LOSS"
else
    echo "Upper bound ($HI_LR) is worse: $HI_LOSS > $LO_LOSS"
    echo "Optimum is bracketed in [$LO_LR, $HI_LR]"
    BEST_LR="$LO_LR"
    BEST_LOSS="$LO_LOSS"

    # Bisect
    for (( r=1; r<=ROUNDS; r++ )); do
        MID_LR=$(midpoint_lr "$LO_LR" "$HI_LR")
        echo ""
        echo "--- Round $r/$ROUNDS: probe midpoint LR=$MID_LR (bracket [$LO_LR, $HI_LR]) ---"

        run_probe "$MID_LR"
        MID_LOSS="$LAST_LOSS"

        if float_lt "$MID_LOSS" "$BEST_LOSS"; then
            echo "New best: LR=$MID_LR loss=$MID_LOSS (was $BEST_LOSS)"
            BEST_LR="$MID_LR"
            BEST_LOSS="$MID_LOSS"
            # Optimum is in [MID, HI] or [LO, MID] — move the worse bound
            LO_LR="$MID_LR"
            LO_LOSS="$MID_LOSS"
        else
            echo "Midpoint worse: $MID_LOSS > $BEST_LOSS — narrowing upper bound"
            HI_LR="$MID_LR"
            HI_LOSS="$MID_LOSS"
        fi
    done
fi

echo ""
echo "========================================"
echo "  BISECTION COMPLETE"
echo "  Best LR: $BEST_LR (probe loss: $BEST_LOSS)"
echo "  Running full $FULL_STEPS-step confirmation..."
echo "========================================"
echo ""

# Confirmation run at full steps with wandb
run_name="sweep_bisect_lr${BEST_LR}_wd01"

if grep -q "^${run_name}	" "$TSV" 2>/dev/null; then
    echo "Already in $TSV — skipping confirmation"
    echo "=== DONE ==="
    exit 0
fi

confirm_start=$(date +%s)

python "$TRAIN_SCRIPT" \
    --config "$CONFIG" \
    --device "$DEVICE" \
    --batch-size "$BATCH" \
    --grad-accum "$ACCUM" \
    --lr "$BEST_LR" \
    --weight-decay "$WD" \
    --beta2 "$BETA2" \
    --max-steps "$FULL_STEPS" \
    --compile \
    --wandb-name "$run_name" \
    2>&1 | tee "/tmp/${run_name}.log"

confirm_end=$(date +%s)
elapsed=$((confirm_end - confirm_start))

final_loss=$(grep "Last 100 avg loss:" "/tmp/${run_name}.log" | awk '{print $NF}')
final_ppl=$(grep "Final PPL:" "/tmp/${run_name}.log" | awk '{print $NF}')

printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$run_name" "$final_loss" "$final_ppl" "$BEST_LR" "$WD" "$BETA2" "auto" "$BATCH" "$ACCUM" "$FULL_STEPS" "$elapsed" \
    >> "$TSV"

echo ""
echo "========================================"
echo "  FINAL RESULT"
echo "  Optimal LR: $BEST_LR"
echo "  Loss: $final_loss  PPL: $final_ppl"
echo "  Logged to $TSV"
echo "========================================"

# Show full leaderboard
echo ""
echo "=== All sweep results ==="
column -t -s $'\t' "$TSV"
