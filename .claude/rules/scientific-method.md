# Scientific Method & Mertonian Norms

This project follows the scientific method. Every experiment is a test of a hypothesis,
not a fishing expedition. These rules enforce rigor based on Merton's norms of science:
communalism, universalism, disinterestedness, and organized skepticism.

## The Four Norms (Applied)

### 1. Communalism — Share Everything

- All findings belong to the project, not to a session. Document so that anyone
  (including future-you with no memory) can understand and reproduce.
- Every experiment must be registered in `training/docs/experiment_registry.md` BEFORE training starts.
- Results, methodology, and failed attempts are all public record in the project docs.

### 2. Universalism — Let the Data Decide

- Judge results by the numbers, not by how much you want them to work.
- A hypothesis is supported or refuted by the data. Do not reinterpret negative results
  as "needs more training" or "probably works at scale" without evidence.
- Same evaluation protocol for every experiment. No special treatment for favored configs.

### 3. Disinterestedness — No Motivated Reasoning

- Pre-register the hypothesis AND the expected outcome BEFORE running.
- If the result contradicts expectation, that's information. Don't rationalize it away.
- Report the number you got, not the number you wanted.
- Negative results get the same documentation quality as positive results.

### 4. Organized Skepticism — Actively Try to Disprove

- After a positive result, ask: "What else could explain this?"
  - LR artifact? (Run the baseline at the same LR)
  - Param count mismatch? (Check overhead percentage)
  - Training duration effect? (Compare at matched steps)
  - Random seed variance? (Note if the delta is small enough to be noise)
- A result is not "confirmed" until the obvious alternative explanations are ruled out.

## Pre-Registration Protocol

BEFORE launching any training or sweep run, create an entry in `training/docs/experiment_registry.md`:

```markdown
### EXP-{number}: {name}
- **Date:** {YYYY-MM-DD}
- **Status:** REGISTERED | RUNNING | COMPLETED | FAILED
- **Hypothesis:** {What you expect to happen and why}
- **Variable:** {The ONE thing that changed vs control}
- **Control:** {What you're comparing against, with its result}
- **Prediction:** {Quantitative — e.g., "expect LR 1e-3 to beat 6e-4 by 5-10% lower loss"}
- **Config:** {model config, HP, hardware, data}
- **Result:** {filled in after run completes}
- **Verdict:** CONFIRMED | REFUTED | INCONCLUSIVE
- **Analysis:** {What happened, why, and what it means}
```

The prediction forces you to think about effect size before running. If you can't
predict a direction, that's fine — say "exploratory, no directional prediction" — but
be honest that you're exploring, not testing.

## Post-Experiment Checklist

After EVERY completed run:

1. Record the result in the registry entry (Status -> COMPLETED)
2. Compare result to prediction — was your mental model right?
3. If result is positive: list alternative explanations and whether they're ruled out
4. If result is negative: what does this tell us about the config/architecture?
5. Update `training/sweep_results.tsv` with the raw numbers
6. Update the findings document with analysis
7. If the result changes any prior conclusions, update those entries too

## Red Flags — Stop and Think

- You're about to run something without a hypothesis -> pre-register first
- You ran 3+ experiments that all confirmed your expectations -> are you testing hard enough?
- You're comparing results from different LRs/steps/batch sizes -> not a fair test
- You're explaining away a negative result -> the data is probably right
- You got benchmark numbers and were about to slap them on an issue without methodology -> stop

## Evaluation Protocol

### Sweep Runs (HP Search)

Standard budgets for the RX 7800 XT with 100M v3:
- **Short directional test:** 1000-2000 optimizer steps (~4000-8000 micro-steps at accum 4)
- **Full sweep run:** 4000+ micro-steps per config
- **Full pretraining:** ~400K micro-steps (1 epoch through 6.5B tokens)

### Metrics

- **Loss** (cross-entropy): Primary metric for pretraining sweeps. Lower is better.
- **PPL** (perplexity): exp(loss). More interpretable for comparison with prior felixlm work.
- **Tokens/sec**: Throughput. Report alongside loss — a 2% loss win at 3x cost may not be worth it.
- **VRAM peak**: Report for batch size experiments.
- Always report ALL metrics, not just the favorable ones.

### Benchmark Metrics (Mnemonic Quality)

- **nDCG@5**: Primary retrieval quality metric (IR benchmark)
- **Precision@5, Recall@5, MRR**: Supporting retrieval metrics
- **JSON compliance rate**: Encoding output validity
- **Latency**: End-to-end response time
- Report against established baselines (Gemini, stub LLM) with exact commands used.

## What Counts as "Confirmed"

A finding is confirmed when:
1. The effect is observed at the predicted scale AND direction
2. The most obvious alternative explanation (usually LR or param mismatch) is ruled out
3. Ideally, the effect holds across multiple runs or conditions

A finding is "promising" (not confirmed) when:
1. Observed under one condition only
2. Delta is small (could be noise)
3. No alternative explanation has been explicitly tested
