# Mnemonic-LM Training Infrastructure

Training pipeline for building a bespoke local LLM to replace cloud LLM dependencies in mnemonic.

## Overview

Mnemonic's LLM tasks are narrow and repetitive: 4 task types, fixed JSON schemas, a controlled vocabulary of ~60 terms. This makes it tractable to train a small, specialized model that matches or exceeds cloud LLM quality for these specific tasks.

## Architecture

| Model | Task | Size | Toolchain |
|-------|------|------|-----------|
| Encoder | Encoding, abstraction, perception (80%+ of calls) | 500M -> 4B | Unsloth |
| Synthesizer | Retrieval synthesis with tool-use | Qwen 3.5 4B | Unsloth |
| Embedder | Vector embeddings | 100-300M | sentence-transformers |

## Directory Structure

```
training/
  data/
    raw/           # Unfiltered captures from InstrumentedProvider (JSONL)
    validated/     # Passed quality gates (JSONL)
    rejected/      # Failed quality gates, kept for analysis (JSONL)
    eval/          # Frozen evaluation set (never trained on)
  scripts/
    validate.py    # Quality gate pipeline
    finetune.py    # Unsloth fine-tuning script
    evaluate.py    # Mnemonic quality score evaluation
    serve.py       # Model serving (vLLM wrapper)
  configs/
    encoder.yaml   # Encoder model training config
    synthesizer.yaml
    embedder.yaml
  checkpoints/     # Model checkpoints (gitignored)
  program.md       # Autoresearch steering instructions
```

## Data Capture

The Go daemon captures training data automatically via the `TrainingDataCapture` wrapper around the LLM provider. Enable it in `config.yaml`:

```yaml
training:
  capture_enabled: true
  capture_dir: "~/.mnemonic/training-data"
```

Captured data flows through quality gates before inclusion in the training set.

## Quality Gates

Every training example must pass:

**Hard gates (auto-reject):**
- Valid JSON matching the encoding schema
- Field constraints (gist <60 chars, summary <100 chars, salience in [0,1])
- Concepts from controlled vocabulary or clearly content-derived
- No empty/placeholder content

**Soft gates (flagged for review):**
- Salience outliers
- Emotional tone mismatches
- Low concept coverage
- Suspiciously similar outputs

## Quick Start

```bash
# 1. Enable data capture in mnemonic
# Edit config.yaml to set training.capture_enabled: true

# 2. Set up Python environment
cd training
python -m venv .venv
source .venv/bin/activate
pip install unsloth torch transformers datasets

# 3. Run quality validation on captured data
python scripts/validate.py

# 4. Fine-tune (once you have enough validated data)
python scripts/finetune.py --config configs/encoder.yaml
```

## Prerequisites

- Python 3.12+
- PyTorch with ROCm support
- Unsloth
- AMD RX 7800 XT (16GB VRAM) or compatible GPU
