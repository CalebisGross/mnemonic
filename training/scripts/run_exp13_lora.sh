#!/bin/bash
# EXP-13: Spokes-only vs Spokes + LoRA
# Tests 2 configurations at 500 steps each
# Requires: GPU free (no LM Studio)

set -e

source ~/Projects/felixlm/.venv/bin/activate
cd ~/Projects/mem

export TORCH_ROCM_AOTRITON_ENABLE_EXPERIMENTAL=1

BASE_CMD="python training/scripts/train_qwen_spokes.py \
  --base-model models/qwen3.5-2b \
  --steps 500 \
  --eval-interval 100 \
  --log-interval 25 \
  --seq-len 512 \
  --device auto \
  --patience 0"

echo "============================================="
echo "  EXP-13: Spokes vs Spokes + LoRA"
echo "  $(date)"
echo "============================================="

# Config A: Spokes only (baseline from EXP-12 Config A)
echo ""
echo ">>> Config A: Spokes only — all 24 layers (25.2M params)"
echo "============================================="
$BASE_CMD \
  --spoke-every-n 1 \
  --checkpoint-dir checkpoints/exp13_spokes_only \
  2>&1 | tee /tmp/exp13_a_spokes.log

# Config B: Spokes + LoRA r16 on Q/V of attention layers
echo ""
echo ">>> Config B: Spokes + LoRA r16 on Q/V (~27.6M params)"
echo "============================================="
$BASE_CMD \
  --spoke-every-n 1 \
  --lora-rank 16 \
  --lora-alpha 32 \
  --checkpoint-dir checkpoints/exp13_spokes_lora \
  2>&1 | tee /tmp/exp13_b_lora.log

# Summary
echo ""
echo "============================================="
echo "  EXP-13 SUMMARY"
echo "============================================="
echo ""
for config in a_spokes b_lora; do
  label=$(echo $config | tr '_' ' ')
  best=$(grep "Best eval loss" /tmp/exp13_${config}.log 2>/dev/null | tail -1)
  final=$(grep "Final eval loss" /tmp/exp13_${config}.log 2>/dev/null | tail -1)
  echo "Config $label:"
  echo "  $best"
  echo "  $final"
  echo ""
done

echo "Done at $(date)"
