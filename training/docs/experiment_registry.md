# Mnemonic-LM Experiment Registry

Pre-registered experiments for Felix-LM v3 100M pretraining on mnemonic's curated data mix.

---

## Baselines

### BASELINE-1: IR Quality Benchmark (Stub LLM)

- **Date:** 2026-03-17
- **Status:** COMPLETED
- **Purpose:** Establish retrieval quality floor using deterministic stub LLM embeddings (128-dim bag-of-words). This measures the retrieval system itself, independent of the LLM.
- **Command:** `go run ./cmd/benchmark-quality/ -compare -report markdown`
- **Commit:** 254d004 (feat/pretrain-hp-sweep)
- **Environment:** Linux x86_64, mnemonic v0.16.0, 6 scenarios, 20 queries, 5 consolidation cycles
- **Results:**

| Approach | P@5 | R@5 | MRR | nDCG |
|----------|-----|-----|-----|------|
| FTS5 (BM25) | 0.390 | 0.821 | 0.842 | 0.758 |
| Vector (Cosine) | 0.330 | 0.688 | 0.758 | 0.625 |
| Hybrid (RRF) | 0.420 | 0.886 | 0.900 | 0.836 |
| Mnemonic (no spread) | 0.400 | 0.853 | 0.842 | 0.786 |
| **Mnemonic (full)** | **0.450** | **0.944** | **0.842** | **0.841** |

- **Analysis:** Mnemonic's full retrieval pipeline (FTS + embeddings + 3-hop spread activation) achieves nDCG 0.841, outperforming industry-standard Hybrid RRF (0.836) by a slim margin. The primary advantage is in recall (0.944 vs 0.886) — spread activation finds memories that keyword + embedding search alone misses. The weakest scenario is "Needle in Haystack" where Mnemonic (full) ties with FTS at nDCG 0.623, suggesting spread activation doesn't help when the target memory has few associations. Strongest scenario is "Associative Recall" (nDCG 0.953) which directly tests the graph traversal. Note: these numbers are with deterministic bag-of-words embeddings, not real LLM embeddings. A trained model producing better embeddings should lift the Vector and Mnemonic approaches while leaving FTS unchanged.

### BASELINE-2: End-to-End Gemini Quality Floor

- **Date:** 2026-03-17
- **Status:** COMPLETED
- **Purpose:** Establish the quality floor that Felix-LM v3 must match or exceed when replacing Gemini as the encoding LLM.
- **Command:** `go run ./cmd/benchmark/`
- **Commit:** 254d004 (feat/pretrain-hp-sweep)
- **Environment:** Linux x86_64, mnemonic v0.16.0 (daemon running via systemd), Gemini API (model configured in ~/.mnemonic/config.yaml), 15 seed memories, 5 retrieval queries
- **Results:**

| Metric | Value |
|--------|-------|
| Ingestion | 15 memories in 20ms (avg 1ms) |
| Encoding | 2061 memories in 3s (avg 1ms) |
| Associations | 5846 total (2.8 per memory) |
| Retrieval precision | 76% avg (4/5 PASS, 1/5 WEAK) |
| Synthesis quality | 5/5 non-empty, 4/5 on-topic |
| Avg query latency | 5.9s |

| Query | Grade | Precision |
|-------|-------|-----------|
| Q1: SQLite decision | PASS | 100% |
| Q2: Error recall | WEAK | 20% |
| Q3: Retrieval mechanism | PASS | 100% |
| Q4: Photos Library error | PASS | 100% |
| Q5: All decisions | PASS | 60% |

- **Analysis:** Gemini achieves 76% average retrieval precision with full encoding + synthesis. The weak point is Q2 (error recall at 20%) — the benchmark injects 15 seed memories into a database with 2061 existing memories (from the live daemon's real usage), so error-related queries compete with real noise. This is actually a realistic test condition. Latency averages 5.9s per query, dominated by Gemini API round-trips — this is the performance Felix-LM must beat (embedded inference should be <100ms). The quality bar for the embedded model: >=76% precision, 5/5 synthesis non-empty. Latency is expected to be dramatically better.

- **Caveat:** This benchmark ran against a live database with 2061 pre-existing memories (mostly desktop noise from watcher). The 15 seed memories competed with real data. A clean-DB benchmark would likely show higher precision but would be less realistic. Both conditions should be tested when evaluating Felix-LM.

### BASELINE-3: IR Quality Benchmark (Real Gemini Embeddings)

- **Date:** 2026-03-17
- **Status:** COMPLETED
- **Purpose:** Run the IR quality benchmark with real Gemini embeddings instead of the deterministic stub. This isolates the effect of LLM embedding quality on retrieval, and establishes the quality floor for the pipeline scenarios (encoding, episoding, dreaming, consolidation, retrieval end-to-end).
- **Command:** `./bin/benchmark-quality --llm --config config.yaml --cycles 5 --report markdown`
- **Commit:** feat/gemini-benchmark-baseline (v0.17.0)
- **Environment:** Linux x86_64, mnemonic v0.17.0, Gemini 3 Flash Preview (chat + gemini-embedding-2-preview), 6 direct scenarios + 3 pipeline scenarios, 5 consolidation cycles
- **Results (Direct Scenarios — pre-ingested memories, Gemini used for query embeddings only):**

| Metric | Stub | Gemini | Delta |
|--------|------|--------|-------|
| Precision@5 | 0.46 | 0.46 | 0% |
| MRR | 0.84 | 0.84 | 0% |
| nDCG | 0.85 | 0.85 | 0% |
| Noise Suppression | 1.00 | 1.00 | 0% |
| Signal Retention | 1.00 | 1.00 | 0% |

- **Results (Pipeline Scenarios — Gemini does full encoding + all agents):**

| Pipeline | Metric | Stub | Gemini | Delta |
|----------|--------|------|--------|-------|
| Full Day | Noise Suppr. | 0.73 | 0.95 | **+30%** |
| Full Day | nDCG | 0.56 | 0.63 | +13% |
| Cross-Pollination | Noise Suppr. | 0.62 | 0.92 | **+48%** |
| Cross-Pollination | nDCG | 0.57 | 0.67 | +18% |
| Noise Storm | Noise Suppr. | 0.85 | 0.91 | +7% |
| Noise Storm | nDCG | 0.97 | 0.95 | -2% |

- **Analysis:** Direct scenario results are identical between stub and Gemini because those scenarios use pre-ingested memories with stub-generated embeddings — Gemini only affects the query embedding, which goes through the same FTS+vector merge pipeline. The real differentiation shows in pipeline scenarios where Gemini handles the full encoding chain. Gemini's primary advantage is noise suppression: +30% on Full Day, +48% on Cross-Pollination. Gemini assigns more meaningful salience scores, letting consolidation's decay + threshold logic more effectively demote irrelevant memories. The nDCG improvement is modest (+13-18%) because FTS5 dominates retrieval in these scenarios (vector search returns 0 results since stub embeddings stored in the DB don't match Gemini query embeddings). A fair vector comparison would require re-embedding all stored memories with Gemini, which the current benchmark architecture doesn't support. The quality bar for Felix-LM: must achieve >= 0.90 noise suppression and >= 0.63 nDCG on pipeline scenarios.

---

## Phase 2: HP Sweep

### EXP-1: Batch Size Preflight

- **Date:** 2026-03-17
- **Status:** COMPLETED
- **Hypothesis:** Binary search will find max safe batch size for v3_mnemonic_100m with torch.compile on RX 7800 XT (16GB VRAM). Based on felixlm v3 100M results, expect max ~14-16.
- **Variable:** Batch size (binary search from 1 to 24)
- **Control:** N/A (preflight, not a training comparison)
- **Prediction:** Max batch 14-16 based on felixlm precedent with similar model at same VRAM
- **Config:** v3_mnemonic_100m, 4 spokes, r64, embed_proj, gradient_checkpointing, torch.compile, bf16 autocast
- **Hardware:** AMD RX 7800 XT 16GB, ROCm, Linux x86_64
- **Result:** Max batch 14, safe (75%) batch 10. Batch 12 passed, 13 passed, 14 passed, 15 OOM, 18 OOM.
- **Verdict:** CONFIRMED — matches felixlm precedent exactly (max 14 on same GPU)
- **Analysis:** Binary search converged in 6 tests (12 OK, 18 OOM, 15 OOM, 13 OK, 14 OK). The 75% safety margin of 10 is conservative; batch 12 has a healthy 2-sample margin below max. Selected batch 12 for sweep runs — maximizes throughput while keeping margin for memory spikes during training (optimizer state accumulation, gradient checkpointing overhead). The preflight script (preflight_batch.py) correctly caught OOM in-process without triggering the Linux OOM killer.

### EXP-2: Phase 1 — LR + Weight Decay Sweep

- **Date:** 2026-03-17
- **Status:** RUNNING
- **Hypothesis:** The optimal LR for v3 100M on our data mix is in the range 6e-4 to 3e-3. At 100M scale with 1B tokens, felixlm found LR 3e-3 optimal. Our run is different: 6.5B tokens (much more data), seq_len 2048 (vs 512), and a curated domain mix (vs Dolma). Longer training generally favors lower peak LR, so we expect the optimum to be lower than 3e-3, likely around 1e-3.
- **Variable:** Learning rate (6e-4, 1e-3, 2e-3) x weight decay (0.1, 0.05)
- **Control:** LR 6e-4 / WD 0.1 (current default from train_mnemonic_lm.py)
- **Prediction:** LR 1e-3 beats 6e-4 by 5-15% lower loss at 4000 micro-steps. WD 0.05 vs 0.1 will show <2% difference (WD matters more in longer runs).
- **Config:** v3_mnemonic_100m, batch 12, accum 22, 4000 micro-steps, torch.compile, wandb group hp_sweep_v3_100m
- **Hardware:** AMD RX 7800 XT 16GB, ROCm, Linux x86_64
- **Result:** (pending)
- **Verdict:** (pending)
- **Analysis:** (pending)
