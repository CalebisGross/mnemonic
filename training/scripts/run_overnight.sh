#!/usr/bin/env bash
# Mnemonic-LM Overnight Data Pipeline
# Downloads all pretraining data sources, cleans, and prepares for tokenization.
#
# Usage:
#   cd ~/Projects/mem/training
#   bash scripts/run_overnight.sh
#
# Skips sources that already have data. Downloads remaining sources in parallel
# where possible. PeS2o is the longest-running download (~hours).

set -euo pipefail

PYTHON="/home/hubcaps/Projects/mem/.venv/bin/python"
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)"
TRAINING_DIR="$(dirname "$SCRIPTS_DIR")"
LOG_DIR="/tmp/mnemonic-lm-logs"

cd "$TRAINING_DIR"
mkdir -p "$LOG_DIR"

echo "============================================"
echo "  Mnemonic-LM Overnight Data Pipeline"
echo "  Started: $(date)"
echo "  Python:  $PYTHON"
echo "  Log dir: $LOG_DIR"
echo "============================================"
echo ""

# Helper: count lines in all JSONL files in a directory
count_docs() {
    local dir="$1"
    if [ -d "$dir" ] && ls "$dir"/*.jsonl &>/dev/null; then
        cat "$dir"/*.jsonl 2>/dev/null | wc -l
    else
        echo 0
    fi
}

# Helper: check if a source needs downloading
needs_download() {
    local dir="$1"
    local min_docs="${2:-1}"
    local docs
    docs=$(count_docs "$dir")
    [ "$docs" -lt "$min_docs" ]
}

PIDS=()
NAMES=()

# --- FineWeb-Edu (18% weight, ~4.1M docs) ---
if needs_download "data/pretrain/fineweb" 1000000; then
    echo "[FineWeb-Edu] Starting download..."
    $PYTHON -u scripts/download_fineweb.py \
        --output-dir data/pretrain/fineweb \
        > "$LOG_DIR/fineweb.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("FineWeb-Edu")
else
    echo "[FineWeb-Edu] Already downloaded ($(count_docs data/pretrain/fineweb) docs)"
fi

# --- StarCoderData (28% weight, ~500K docs) ---
if needs_download "data/pretrain/code" 100000; then
    echo "[Code] Starting download..."
    $PYTHON -u scripts/download_code.py \
        --output-dir data/pretrain/code \
        --total-budget 500000 \
        > "$LOG_DIR/code.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("Code")
else
    echo "[Code] Already downloaded ($(count_docs data/pretrain/code) docs)"
fi

# --- PeS2o Academic Papers (32% weight combined — most critical) ---
if needs_download "data/pretrain/pes2o_neuro" 100000 || needs_download "data/pretrain/pes2o_cs" 50000; then
    echo "[PeS2o] Starting download (neuro + CS papers)..."
    $PYTHON -u scripts/download_pes2o.py \
        --output-dir data/pretrain \
        --max-neuro 500000 \
        --max-cs 200000 \
        > "$LOG_DIR/pes2o.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("PeS2o")
else
    echo "[PeS2o] Already downloaded (neuro: $(count_docs data/pretrain/pes2o_neuro), CS: $(count_docs data/pretrain/pes2o_cs))"
fi

# --- StackOverflow (8% weight) ---
if needs_download "data/pretrain/stackoverflow" 50000; then
    echo "[StackOverflow] Starting download..."
    $PYTHON -u scripts/download_stackoverflow.py \
        --output-dir data/pretrain/stackoverflow \
        --max-docs 200000 \
        --min-score 3 \
        > "$LOG_DIR/stackoverflow.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("StackOverflow")
else
    echo "[StackOverflow] Already downloaded ($(count_docs data/pretrain/stackoverflow) docs)"
fi

# --- CommitPackFT (5% weight) ---
if needs_download "data/pretrain/commits" 20000; then
    echo "[Commits] Starting download..."
    $PYTHON -u scripts/download_commits.py \
        --output-dir data/pretrain/commits \
        --max-docs 100000 \
        > "$LOG_DIR/commits.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("Commits")
else
    echo "[Commits] Already downloaded ($(count_docs data/pretrain/commits) docs)"
fi

# --- JSON/Structured (5% weight) ---
if needs_download "data/pretrain/json_structured" 10000; then
    echo "[JSON] Starting download..."
    $PYTHON -u scripts/download_json.py \
        --output-dir data/pretrain/json_structured \
        --max-docs 50000 \
        > "$LOG_DIR/json.log" 2>&1 &
    PIDS+=($!)
    NAMES+=("JSON")
else
    echo "[JSON] Already downloaded ($(count_docs data/pretrain/json_structured) docs)"
fi

echo ""
echo "Running ${#PIDS[@]} downloads in parallel..."
echo "  Monitor logs: tail -f $LOG_DIR/*.log"
echo ""

# Wait for all downloads, reporting as each finishes
FAILED=0
for i in "${!PIDS[@]}"; do
    pid=${PIDS[$i]}
    name=${NAMES[$i]}
    if wait "$pid"; then
        echo "  [DONE] $name (PID $pid)"
    else
        echo "  [FAIL] $name (PID $pid) — check $LOG_DIR/$(echo "$name" | tr '[:upper:]' '[:lower:]' | tr ' ' '_').log"
        FAILED=$((FAILED + 1))
    fi
done

# --- Summary ---
echo ""
echo "============================================"
echo "  Download Pipeline Complete"
echo "  Finished: $(date)"
if [ "$FAILED" -gt 0 ]; then
    echo "  WARNING: $FAILED download(s) failed!"
fi
echo "============================================"
echo ""
echo "Source sizes:"
for dir in data/pretrain/*/; do
    if [ -d "$dir" ] && [ "$(basename "$dir")" != "tokenized" ]; then
        NAME=$(basename "$dir")
        SIZE=$(du -sh "$dir" | cut -f1)
        DOCS=$(count_docs "$dir")
        echo "  $NAME: $SIZE ($DOCS docs)"
    fi
done
echo ""
echo "Total:"
du -sh data/pretrain/ 2>/dev/null
echo ""
echo "Next steps:"
echo "  1. Train custom tokenizer: $PYTHON scripts/train_tokenizer.py"
echo "  2. Tokenize all sources:   $PYTHON scripts/tokenize_source.py --source <name>"
