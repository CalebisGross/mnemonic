# Handoff: Qwen 3.5 + Spoke Architecture for Mnemonic's Built-In LLM

## Context

Mnemonic is a local-first semantic memory daemon (Go binary) with an embedded LLM engine via llama.cpp (CGo bridge). It needs a small, fast model that can encode arbitrary text into structured JSON (gist, summary, concepts, salience, etc.) and eventually handle synthesis (multi-memory summarization with tool use).

We spent ~3 weeks training Felix-LM v3 100M from scratch (hub-and-spoke architecture, 6.5B token pretrain, custom tokenizer, custom llama.cpp fork). The 100M model cannot generalize the encoding task to novel inputs — it either hallucinated a single memorized template (EXP-9) or produced degenerate repetitive output (EXP-10). The last fine-tuning run (EXP-10, 14K examples, 5 epochs) also had a critical LR error (used pretrain LR 3.5e-3 instead of fine-tune LR ~3.5e-5), wasting 20h of GPU time. The 100M scale is capacity-limited for this task.

The new plan: start from Qwen 3.5 (0.8B or 4B), add Felix's spoke architecture, fine-tune on mnemonic's encoding/compression data, and deploy via llama.cpp with aggressive quantization (TurboQuant for KV cache).

## What Exists

### Codebase
- **Mnemonic repo**: `~/Projects/mem` (Go daemon + training scripts)
- **Felix-LM**: `~/Projects/felixlm` (custom architecture, spoke implementation)
- **Nanochat fork**: `~/Projects/nanochat` (Karpathy's nanochat with spoke integration already done)
- **Compressed protocol**: `~/Projects/compressed-protocol` (compression experiment, has training data)
- **llama.cpp fork**: `appsprout-dev/llama.cpp` (felix branch, has spoke tensor support)

### Spoke Architecture (Felix-LM v3)
Source of truth: `~/Projects/felixlm/felix_lm/v3/spokes.py`

Each spoke layer sits after a transformer block and does:
1. RMSNorm the hidden state
2. For each of S spokes: project down (W_down: d→r), SiLU, project up (W_up: r→d)
3. Average all spoke updates
4. Gate into residual stream: `h = h + sigmoid(gate_bias) * mean_update`

Key properties:
- W_up initialized to zeros (spokes start as identity)
- Progressive gate init: early layers ~0.12, late layers ~0.88
- Agreement (pairwise cosine similarity of spoke views) is a diagnostic signal
- Param overhead: S * 2 * d * r per layer (small — ~5% for typical configs)
- At 100M scale, late-layer spokes routed 91-99% of signal

### Nanochat Spoke Integration (already done)
`~/Projects/nanochat/nanochat/gpt.py` has a working SpokeLayer class integrated into nanochat's GPT model. Key design decisions documented in `~/.claude/plans/cheeky-hugging-bubble.md`:
- SpokeLayer uses parameterless norm (nanochat style)
- Spokes stored as nn.ModuleDict on model (not inside Block) for optimizer routing
- Spoke W_down/W_up → Muon optimizer, gate_bias → AdamW
- Spoke params properly counted in FLOPs and scaling params
- GGUF export script at `~/Projects/nanochat/scripts/export_gguf.py`

### Training Data
- `training/training/data/finetune_full/`: 14,082 train / 1,564 eval (encoding task, pre-tokenized with Felix's 32K BPE tokenizer — will need re-tokenization for Qwen's tokenizer)
- `~/Projects/compressed-protocol/compression_analysis.json`: (passage, compressed_representation, accuracy) triples from Qwen 3.5 4B compression experiments. Filter for >90% accuracy at 2-3x compression.
- `training/data/synthesis_data.jsonl`: 225 synthesis examples (multi-turn with tool use)
- Raw source: 25,426 captures validated down to ~15,656 via `training/scripts/validate.py`

### Existing Infrastructure
- **GPU**: AMD RX 7800 XT (16GB VRAM), ROCm 6.3
- **llama.cpp CGo bridge**: `~/Projects/mem/internal/llm/llamacpp/` — bridges Go daemon to llama.cpp
- **Felix llama.cpp fork**: Has spoke tensor loading (`blk.{N}.spoke.*`) and metadata (`felix.num_spokes`, `felix.spoke_rank`)
- **GBNF grammar**: Constrains JSON output structure during inference. Stored in daemon config.
- **Eval scripts**: `training/scripts/eval_encoding.py` (perplexity + generation modes), `training/scripts/validate.py` (quality gates)
- **Felix-LM venv**: `~/Projects/felixlm/.venv` — has PyTorch 2.9.1+ROCm 6.3, all training deps

### Key Prior Results
- **Felix 100M pretrain**: 34.4h, 6.5B tokens, loss 2.512, PPL 12.3 (EXP-1)
- **Felix 100M fine-tune (EXP-9)**: 3K examples, 3 epochs, eval loss 0.522 — hallucinated on novel inputs
- **Felix 100M fine-tune (EXP-10)**: 14K examples, 5 epochs, eval loss 1.119 — degenerate output on novel inputs. LR was wrong (3.5e-3 pretrain LR, should have been ~3.5e-5). Catastrophic forgetting.
- **Compressed protocol**: Qwen 3.5 4B achieved 94.1% accuracy at 2-3x compression via prompting, but couldn't escape English-adjacent notation without gradient fine-tuning
- **Embedding fine-tune (EXP-7)**: nomic-embed-text-v2-moe fine-tuned on 50K triplets, nDCG@5 improved from 0.499 to 0.882 (+76.8%)

## The Plan

### Phase 1: Qwen 3.5 Base Selection and Spoke Integration

**Choose base model**: Qwen 3.5 0.8B vs 4B.
- 0.8B: Easy to train on RX 7800 XT (fits in 16GB with full fine-tune), fast inference (~1-2s encoding), ~500MB quantized. Probably sufficient for structured encoding.
- 4B: More capable (better at synthesis/reasoning), but needs gradient checkpointing or LoRA for training on 16GB. Inference ~5-10s. ~2-3GB quantized.
- Recommendation: Start with 0.8B for encoding, evaluate whether it's sufficient before considering 4B.

**Add spokes to Qwen's architecture**:
- Qwen 3.5 uses standard transformer blocks (RMSNorm, GQA, SwiGLU MLP)
- Add SpokeLayer after each transformer block, same as Felix/nanochat pattern
- For Qwen 0.8B (d_model=1024, 24 layers): spoke_rank=64, num_spokes=4 adds ~12.6M params (1.5% overhead)
- For Qwen 4B (d_model=2560, 36 layers): spoke_rank=128, num_spokes=4 adds ~94.4M params (2.4% overhead)

**Training strategy**:
- Freeze Qwen base weights
- Train only spoke params (W_down, W_up, gate_bias) — this is ~12-95M trainable params
- Optionally add LoRA on attention Q/V for task adaptation (rank 16-32)
- **CRITICAL: Fine-tune LR must be 10-100x below any pretrain LR. For spoke-only training on a frozen base, start with LR 1e-4 to 5e-4. NEVER use pretrain-scale LR (1e-3+) for fine-tuning. Review ALL hyperparameters before launching any GPU run.**

### Phase 2: Training on Encoding + Compression Data

- Re-tokenize all training data with Qwen's tokenizer (not Felix's 32K BPE)
- Fine-tune spokes (+ optional LoRA) on encoding task first
- Then add compression protocol data for the non-human encoding capability
- Use nanochat's Muon optimizer for spoke matrices, AdamW for gate scalars
- Systematic HP search via autoresearch methodology (pre-register experiments, controlled variables)

### Phase 3: Quantization and Deployment

- Export to GGUF (Qwen architecture is already supported in llama.cpp)
- If spokes are present, use the felix llama.cpp fork's tensor naming convention
- Apply TurboQuant for KV cache compression (3-4 bit, training-free, 6x memory reduction)
  - TurboQuant paper: March 25, 2026, Google Research
  - llama.cpp discussion: https://github.com/ggml-org/llama.cpp/discussions/20969
  - Uses PolarQuant (polar coordinate conversion) + Quantized Johnson-Lindenstrauss
  - No training required — applies at inference time
- Weight quantization: Q4_K_M or Q8_0 depending on quality requirements
- Wire into mnemonic's existing CGo bridge (`internal/llm/llamacpp/`)

### Phase 4: Evaluation

- Use existing eval pipeline (`training/scripts/eval_encoding.py`)
- Test on novel inputs (the hallucination test that caught Felix's failures)
- Benchmark against Gemini Flash (the current cloud fallback)
- Measure: JSON validity rate, schema compliance, concept Jaccard vs ground truth, salience accuracy, inference latency

## Open Questions

1. **Spoke vs LoRA vs both**: Spokes give routing/gating diagnostics and the agreement signal. LoRA is simpler and doesn't need a custom llama.cpp fork. Could do both (LoRA on base + spokes for routing). Need to decide before implementing.

2. **llama.cpp spoke support for Qwen**: The existing felix fork handles Felix's architecture. Qwen + spokes would need the spoke tensor loading grafted onto Qwen's architecture handler in llama.cpp. Alternatively, merge LoRA weights into base and skip spokes entirely for a simpler deployment path.

3. **Tokenizer mismatch**: All existing training data is tokenized with Felix's 32K BPE. Qwen uses a different tokenizer (~150K vocab). Need to re-tokenize from raw text, not from the pre-tokenized JSONL files.

4. **0.8B vs 4B**: Start with 0.8B. If encoding quality is sufficient, ship it. If not, scale to 4B.

5. **TurboQuant maturity**: Brand new (March 25). The llama.cpp discussion is open but no merged implementation. Worth watching and integrating when available, but don't block on it.

## Constraints

- **Single GPU**: AMD RX 7800 XT, 16GB VRAM, ROCm 6.3. Training runs block all other GPU work.
- **Training time is precious**: Every run must be pre-registered with hypothesis, HPs reviewed against baselines, and expected outcomes documented BEFORE launching.
- **Python 3.12.3**: Do NOT suggest upgrading — PyTorch ROCm is incompatible with 3.14.
- **Felix-LM venv**: `source ~/Projects/felixlm/.venv/bin/activate` — has PyTorch 2.9.1+ROCm, all deps.
- **Scientific method**: All experiments pre-registered in `training/docs/experiment_registry.md`. See `.claude/rules/scientific-method.md` and `.claude/rules/experiment-logging.md`.

## First Steps

1. Download Qwen 3.5 0.8B weights (HuggingFace)
2. Write a SpokeLayer adapter that wraps Qwen's transformer blocks (reuse the pattern from nanochat's gpt.py)
3. Re-tokenize encoding training data with Qwen's tokenizer
4. Pre-register EXP-11 in experiment registry
5. Short smoke test: freeze base, train spokes only, 100 steps, verify loss decreases and JSON output is valid
6. Full fine-tune run with proper LR (~1e-4 to 5e-4) — should be ~2-4h, not 20h
