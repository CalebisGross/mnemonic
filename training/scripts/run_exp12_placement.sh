#!/bin/bash
# EXP-12: Spoke placement on hybrid architecture
# Tests 4 configurations at 500 steps each
# Requires: GPU free (no LM Studio), ~30 min total at seq_len 512

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
echo "  EXP-12: Spoke Placement Experiments"
echo "  $(date)"
echo "============================================="

# Config A: All 24 layers (baseline)
echo ""
echo ">>> Config A: All 24 layers (25.2M params)"
echo "============================================="
$BASE_CMD \
  --spoke-every-n 1 \
  --checkpoint-dir checkpoints/exp12_all_layers \
  2>&1 | tee /tmp/exp12_a_all.log

# Config B: Attention-only (6 layers)
echo ""
echo ">>> Config B: Attention-only — layers 3,7,11,15,19,23 (6.3M params)"
echo "============================================="
$BASE_CMD \
  --spoke-layers 3,7,11,15,19,23 \
  --checkpoint-dir checkpoints/exp12_attn_only \
  2>&1 | tee /tmp/exp12_b_attn.log

# Config C: Delta-net-only (18 layers)
echo ""
echo ">>> Config C: Delta-net-only — 18 layers (18.9M params)"
echo "============================================="
$BASE_CMD \
  --spoke-layers 0,1,2,4,5,6,8,9,10,12,13,14,16,17,18,20,21,22 \
  --checkpoint-dir checkpoints/exp12_delta_only \
  2>&1 | tee /tmp/exp12_c_delta.log

# Config D: Every-other (12 layers)
echo ""
echo ">>> Config D: Every-other — 12 layers (12.6M params)"
echo "============================================="
$BASE_CMD \
  --spoke-layers 0,2,4,6,8,10,12,14,16,18,20,22 \
  --checkpoint-dir checkpoints/exp12_every_other \
  2>&1 | tee /tmp/exp12_d_every_other.log

# Summary
echo ""
echo "============================================="
echo "  EXP-12 SUMMARY"
echo "============================================="
echo ""
for config in a_all b_attn c_delta d_every_other; do
  label=$(echo $config | tr '_' ' ')
  best=$(grep "Best eval loss" /tmp/exp12_${config}.log 2>/dev/null | tail -1)
  final=$(grep "Final eval loss" /tmp/exp12_${config}.log 2>/dev/null | tail -1)
  echo "Config $label:"
  echo "  $best"
  echo "  $final"
  echo ""
done

echo "Done at $(date)"
