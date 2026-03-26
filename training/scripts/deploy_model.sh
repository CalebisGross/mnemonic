#!/bin/bash
# Deploy a fine-tuned Felix-LM checkpoint to the mnemonic daemon.
#
# Chains: export GGUF -> quantize Q8_0 -> copy to models/ -> rebuild daemon -> restart
#
# Usage:
#   ./training/scripts/deploy_model.sh checkpoints/v3_mnemonic_100m_ft/last.pt
#   ./training/scripts/deploy_model.sh checkpoints/v3_mnemonic_100m_ft/last.pt --name encoder-v2
#
# Requires: Felix-LM venv active, llama-quantize built

set -euo pipefail

CHECKPOINT="${1:?Usage: deploy_model.sh <checkpoint.pt> [--name <model-name>]}"
MODEL_NAME="felix-encoder-v1"

# Parse optional --name arg
shift
while [[ $# -gt 0 ]]; do
    case $1 in
        --name) MODEL_NAME="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
MODELS_DIR="$PROJECT_DIR/models"
GGUF_F16="$MODELS_DIR/${MODEL_NAME}.gguf"
GGUF_Q8="$MODELS_DIR/${MODEL_NAME}-q8_0.gguf"
QUANTIZE_BIN="$PROJECT_DIR/third_party/llama.cpp/build/bin/llama-quantize"

echo "=== Deploy Model Pipeline ==="
echo "  Checkpoint: $CHECKPOINT"
echo "  Model name: $MODEL_NAME"
echo "  Output F16: $GGUF_F16"
echo "  Output Q8:  $GGUF_Q8"
echo ""

# Step 1: Export GGUF
echo "--- Step 1: Export GGUF ---"
python3 "$SCRIPT_DIR/export_gguf.py" \
    --checkpoint "$CHECKPOINT" \
    --output "$GGUF_F16" \
    --tokenizer "$PROJECT_DIR/training/tokenizer"

ls -lh "$GGUF_F16"
echo ""

# Step 2: Quantize to Q8_0
echo "--- Step 2: Quantize to Q8_0 ---"
if [ ! -f "$QUANTIZE_BIN" ]; then
    echo "llama-quantize not found. Building CPU-only..."
    cmake -B /tmp/llama-quantize-build -S "$PROJECT_DIR/third_party/llama.cpp" \
        -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=ON 2>&1 | tail -1
    cmake --build /tmp/llama-quantize-build --target llama-quantize -j"$(nproc)" 2>&1 | tail -1
    QUANTIZE_BIN="/tmp/llama-quantize-build/bin/llama-quantize"
fi

"$QUANTIZE_BIN" "$GGUF_F16" "$GGUF_Q8" q8_0

ls -lh "$GGUF_Q8"
echo ""

# Step 3: Rebuild daemon with embedded model
echo "--- Step 3: Rebuild daemon ---"
cd "$PROJECT_DIR"
ROCM=1 make build-embedded 2>&1 | tail -1
echo ""

# Step 4: Restart daemon
echo "--- Step 4: Restart daemon ---"
systemctl --user restart mnemonic
sleep 2
systemctl --user status mnemonic --no-pager | head -8
echo ""

# Step 5: Verify
echo "--- Step 5: Verify ---"
curl -s http://127.0.0.1:9999/api/v1/health | python3 -m json.tool 2>/dev/null | head -5
echo ""

echo "=== Deploy Complete ==="
echo "  F16 model: $GGUF_F16 ($(du -h "$GGUF_F16" | cut -f1))"
echo "  Q8_0 model: $GGUF_Q8 ($(du -h "$GGUF_Q8" | cut -f1))"
echo ""
echo "To use embedded provider, add to config.yaml:"
echo "  llm:"
echo "    provider: embedded"
echo "    embedded:"
echo "      models_dir: $MODELS_DIR"
echo "      chat_model_file: ${MODEL_NAME}-q8_0.gguf"
