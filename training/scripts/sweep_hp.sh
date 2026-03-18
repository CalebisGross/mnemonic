#!/bin/bash
# Hyperparameter sweep for mnemonic-lm v3 100M pretraining.
#
# Usage:
#   ./training/scripts/sweep_hp.sh --phase 0        # Batch size test
#   ./training/scripts/sweep_hp.sh --phase 1        # LR + WD sweep
#   ./training/scripts/sweep_hp.sh --phase 2        # Beta2 sweep (reads Phase 1 results)
#   ./training/scripts/sweep_hp.sh --phase 3        # Warmup sweep (reads Phase 2 results)
#   ./training/scripts/sweep_hp.sh --dry-run --phase 1  # Preview commands
#
# Results written to training/sweep_results.tsv
# All runs logged to wandb group "hp_sweep_v3_100m"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

# --- Configuration ---
VENV_ACTIVATE="source $HOME/Projects/felixlm/.venv/bin/activate"
TRAIN_SCRIPT="training/scripts/train_mnemonic_lm.py"
RESULTS_FILE="training/sweep_results.tsv"
LOG_DIR="/tmp/mnemonic-lm-logs/sweep"
WANDB_GROUP="hp_sweep_v3_100m"
CONFIG="v3_mnemonic_100m"
SWEEP_STEPS=4000          # micro-steps per sweep run (1000 optimizer steps at accum=4)
SAVE_INTERVAL=2000        # checkpoint midway through sweep run
COOLDOWN=30               # seconds between runs

# Defaults (overridden per phase)
BATCH=8
ACCUM=4
COMPILE="--compile"

# --- Parse args ---
PHASE=""
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --phase) PHASE="$2"; shift 2 ;;
        --dry-run) DRY_RUN=true; shift ;;
        --batch) BATCH="$2"; shift 2 ;;
        --accum) ACCUM="$2"; shift 2 ;;
        --steps) SWEEP_STEPS="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

if [[ -z "$PHASE" ]]; then
    echo "Usage: $0 --phase {0|1|2|3} [--dry-run] [--batch N] [--accum N] [--steps N]"
    exit 1
fi

mkdir -p "$LOG_DIR"

# --- Results file ---
init_results() {
    if [[ ! -f "$RESULTS_FILE" ]]; then
        printf "run_name\tfinal_loss\tfinal_ppl\tlr\twd\tbeta2\twarmup\tbatch\taccum\tsteps\ttime_s\n" > "$RESULTS_FILE"
    fi
}

# --- Run a single experiment ---
run_experiment() {
    local name="$1"; shift
    local extra_args=("$@")

    local cmd="PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True python -u $TRAIN_SCRIPT"
    cmd+=" --config $CONFIG --device cuda"
    cmd+=" --max-steps $SWEEP_STEPS --save-interval $SAVE_INTERVAL"
    cmd+=" --ckpt-dir checkpoints/$name"
    cmd+=" --run-name $name --wandb-group $WANDB_GROUP --wandb-tags sweep"
    cmd+=" $COMPILE"
    cmd+=" ${extra_args[*]}"

    echo ""
    echo "================================================================"
    echo "  RUN: $name"

    # Skip if already completed (result in TSV)
    if grep -qP "^${name}\t" "$RESULTS_FILE" 2>/dev/null; then
        local prev_loss
        prev_loss=$(awk -F'\t' -v n="$name" '$1 == n && $2 != "FAILED" {print $2}' "$RESULTS_FILE")
        if [[ -n "$prev_loss" ]]; then
            echo "  SKIPPED — already completed (loss=$prev_loss)"
            echo "================================================================"
            return
        fi
        # Previous run FAILED — remove entry and retry
        echo "  Previous run FAILED, retrying..."
        sed -i "/^${name}\t/d" "$RESULTS_FILE"
    fi

    # Auto-resume if checkpoint exists for THIS run (crashed mid-run)
    local ckpt_dir="checkpoints/${name}"
    if [[ -f "$ckpt_dir/last.pt" ]]; then
        echo "  Found checkpoint — adding --resume last.pt"
        cmd+=" --resume $ckpt_dir/last.pt"
    fi

    echo "  CMD: $cmd"
    echo "================================================================"

    if $DRY_RUN; then
        echo "  [DRY RUN — skipped]"
        return
    fi

    local log_file="$LOG_DIR/${name}.log"
    local start_ts
    start_ts=$(date +%s)

    # Run training, capture output
    eval "$VENV_ACTIVATE && $cmd" 2>&1 | tee "$log_file"
    local exit_code=${PIPESTATUS[0]}

    local end_ts
    end_ts=$(date +%s)
    local elapsed=$((end_ts - start_ts))

    if [[ $exit_code -ne 0 ]]; then
        echo "  FAILED (exit code $exit_code)"
        printf "%s\tFAILED\tFAILED\t-\t-\t-\t-\t-\t-\t%s\t%s\n" \
            "$name" "$SWEEP_STEPS" "$elapsed" >> "$RESULTS_FILE"
        return
    fi

    # Parse results from training output
    local final_loss final_ppl
    final_loss=$(grep "Last 100 avg loss:" "$log_file" | tail -1 | grep -oP '[\d.]+$' || echo "N/A")
    final_ppl=$(grep "Final PPL:" "$log_file" | tail -1 | grep -oP '[\d.]+$' || echo "N/A")

    # Parse hyperparams from the run args
    local lr wd beta2 warmup batch accum
    lr=$(echo "${extra_args[*]}" | grep -oP '(?<=--lr )\S+' || echo "6e-4")
    wd=$(echo "${extra_args[*]}" | grep -oP '(?<=--weight-decay )\S+' || echo "0.1")
    beta2=$(echo "${extra_args[*]}" | grep -oP '(?<=--beta2 )\S+' || echo "0.95")
    warmup=$(echo "${extra_args[*]}" | grep -oP '(?<=--warmup-steps )\S+' || echo "auto")
    batch=$(echo "${extra_args[*]}" | grep -oP '(?<=--batch-size )\S+' || echo "$BATCH")
    accum=$(echo "${extra_args[*]}" | grep -oP '(?<=--grad-accum )\S+' || echo "$ACCUM")

    printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
        "$name" "$final_loss" "$final_ppl" "$lr" "$wd" "$beta2" "$warmup" \
        "$batch" "$accum" "$SWEEP_STEPS" "$elapsed" >> "$RESULTS_FILE"

    echo ""
    echo "  Result: loss=$final_loss  ppl=$final_ppl  time=${elapsed}s"
    echo "  Log: $log_file"
    echo "  Cooling down ${COOLDOWN}s..."
    sleep "$COOLDOWN"
}

# --- Find best run from results ---
best_from_results() {
    local pattern="$1"
    # Find run matching pattern with lowest loss (skip header, skip FAILED)
    awk -F'\t' -v pat="$pattern" 'NR>1 && $1 ~ pat && $2 != "FAILED" {print $2, $1}' "$RESULTS_FILE" \
        | sort -n | head -1 | awk '{print $2}'
}

best_loss_from_results() {
    local pattern="$1"
    awk -F'\t' -v pat="$pattern" 'NR>1 && $1 ~ pat && $2 != "FAILED" {print $2, $1}' "$RESULTS_FILE" \
        | sort -n | head -1 | awk '{print $1}'
}

get_field() {
    local run_name="$1" field_num="$2"
    awk -F'\t' -v name="$run_name" 'NR>1 && $1 == name {print $'$field_num'}' "$RESULTS_FILE"
}

# --- Phase 0: Batch size preflight ---
# Delegates to preflight_batch.py which does in-process binary search
# with try/except torch.OutOfMemoryError — safe, no OOM killer risk.
phase_0() {
    echo "=== PHASE 0: Batch Size Preflight ==="
    echo "In-process binary search (safe — catches OOM in Python, no system crash)"

    local preflight_script
    preflight_script="$(dirname "$TRAIN_SCRIPT")/preflight_batch.py"

    local cmd="PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True python -u $preflight_script"
    cmd+=" --config $CONFIG --steps 5 --max-try 24 --safety-margin 0.75"
    if [[ -n "$COMPILE" ]]; then
        cmd+=" --compile"
    fi

    if $DRY_RUN; then
        echo "  CMD: $cmd"
        echo "  [DRY RUN — skipped]"
        echo ""
        echo "Use --batch N for subsequent phases after running for real."
        return
    fi

    echo ""
    eval "$VENV_ACTIVATE && $cmd"
    echo ""
    echo "Use the safe batch size above: ./training/scripts/sweep_hp.sh --phase 1 --batch <safe>"
}

# --- Phase 1: LR + Weight Decay sweep ---
phase_1() {
    echo "=== PHASE 1: Learning Rate + Weight Decay Sweep ==="
    echo "5 runs, $SWEEP_STEPS micro-steps each, batch=$BATCH accum=$ACCUM"
    init_results

    run_experiment "sweep_lr6e4_wd01" \
        --lr 6e-4 --weight-decay 0.1 --beta2 0.95 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_lr1e3_wd01" \
        --lr 1e-3 --weight-decay 0.1 --beta2 0.95 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_lr2e3_wd01" \
        --lr 2e-3 --weight-decay 0.1 --beta2 0.95 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_lr3e3_wd01" \
        --lr 3e-3 --weight-decay 0.1 --beta2 0.95 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_lr3e3_wd0" \
        --lr 3e-3 --weight-decay 0.0 --beta2 0.95 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    echo ""
    echo "=== PHASE 1 RESULTS ==="
    column -t -s$'\t' "$RESULTS_FILE"

    local best
    best=$(best_from_results "sweep_lr")
    echo ""
    echo "Best run: $best (loss=$(best_loss_from_results 'sweep_lr'))"
    echo ""
    echo "Next: review results, then run --phase 2 --batch $BATCH"
}

# --- Phase 2: Beta2 + LR interaction ---
phase_2() {
    echo "=== PHASE 2: Beta2 + LR Interaction ==="
    init_results

    # Find best LR from Phase 1
    local best_run best_lr
    best_run=$(best_from_results "sweep_lr")
    if [[ -z "$best_run" ]]; then
        echo "ERROR: No Phase 1 results found. Run --phase 1 first."
        exit 1
    fi
    best_lr=$(get_field "$best_run" 4)
    echo "Best LR from Phase 1: $best_lr (run: $best_run)"

    # Compute 1.5x LR for interaction test
    local higher_lr
    higher_lr=$(python3 -c "print(f'{float('$best_lr') * 1.5:.1e}')")
    echo "Higher LR for interaction test: $higher_lr"
    echo "3 runs, $SWEEP_STEPS micro-steps each"

    run_experiment "sweep_b2_99_lr${best_lr}" \
        --lr "$best_lr" --weight-decay 0.1 --beta2 0.99 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_b2_98_lr${best_lr}" \
        --lr "$best_lr" --weight-decay 0.1 --beta2 0.98 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_b2_99_lr${higher_lr}" \
        --lr "$higher_lr" --weight-decay 0.1 --beta2 0.99 \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    echo ""
    echo "=== PHASE 2 RESULTS ==="
    column -t -s$'\t' "$RESULTS_FILE"

    local best
    best=$(best_from_results "sweep_b2")
    echo ""
    echo "Best beta2 run: $best (loss=$(best_loss_from_results 'sweep_b2'))"
    echo "Compare against Phase 1 best: $best_run (loss=$(best_loss_from_results 'sweep_lr'))"
    echo ""
    echo "Next: review results, then run --phase 3 --batch $BATCH"
}

# --- Phase 3: Warmup validation ---
phase_3() {
    echo "=== PHASE 3: Warmup Validation ==="
    init_results

    # Find overall best from Phase 1+2
    local best_run best_lr best_beta2
    best_run=$(best_from_results "sweep_")
    if [[ -z "$best_run" ]]; then
        echo "ERROR: No prior results found."
        exit 1
    fi
    best_lr=$(get_field "$best_run" 4)
    best_beta2=$(get_field "$best_run" 6)
    echo "Best config so far: LR=$best_lr, beta2=$best_beta2 (run: $best_run)"
    echo "2 runs, $SWEEP_STEPS micro-steps each"

    # Compute warmup values
    local opt_steps=$((SWEEP_STEPS / ACCUM))
    local warmup_5pct=$((opt_steps * 5 / 100))
    local warmup_fixed=2000

    # Note: these test different warmup; auto 10% is already tested in prior phases
    run_experiment "sweep_warmup_5pct" \
        --lr "$best_lr" --weight-decay 0.1 --beta2 "$best_beta2" \
        --warmup-steps "$warmup_5pct" \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    run_experiment "sweep_warmup_2000" \
        --lr "$best_lr" --weight-decay 0.1 --beta2 "$best_beta2" \
        --warmup-steps "$warmup_fixed" \
        --batch-size "$BATCH" --grad-accum "$ACCUM" --spoke-lr-mult 2.0

    echo ""
    echo "=== ALL RESULTS ==="
    column -t -s$'\t' "$RESULTS_FILE"

    local best
    best=$(best_from_results "sweep_")
    echo ""
    echo "=== OVERALL WINNER ==="
    echo "Run: $best"
    echo "Loss: $(best_loss_from_results 'sweep_')"
    echo "LR: $(get_field "$best" 4)  WD: $(get_field "$best" 5)  Beta2: $(get_field "$best" 6)  Warmup: $(get_field "$best" 7)"
    echo ""
    echo "Next: run confirmation at 20K steps with these hyperparameters."
}

# --- Dispatch ---
case "$PHASE" in
    0) phase_0 ;;
    1) phase_1 ;;
    2) phase_2 ;;
    3) phase_3 ;;
    *) echo "Unknown phase: $PHASE (use 0, 1, 2, or 3)"; exit 1 ;;
esac
