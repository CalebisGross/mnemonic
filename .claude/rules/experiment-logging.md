# Experiment Logging

This is a serious research project. Mnemonic-LM training docs are scientific documents, not scratch pads.

## Document Structure

`training/docs/experiments.md` follows a fixed structure. Do not deviate:

1. **Overview** — research question and project goals
2. **Experimental Protocol** — training setup, hardware, data mix, evaluation metrics
3. **Baselines** — Gemini quality floor, stub-LLM IR benchmarks, and what they represent
4. **HP Sweep Results** — grouped by variable (LR, batch size, beta2, warmup, etc.)
5. **Pretraining Runs** — full training results with loss curves and checkpoints
6. **Planned Experiments** — with hypothesis and motivation
7. **Summary** — results table, key findings, open questions

## Quality Bar for Experiment Entries

Every experiment entry MUST include ALL of the following:

- **Header line:** Experiment name, date, config, hardware — on one line
- **Control and variable:** Explicitly state what is being compared and what single variable changed
- **Results table:** Loss and PPL at minimum. Throughput (tokens/sec) for batch experiments.
- **Analysis paragraph:** Not bullet points. A proper paragraph explaining:
  - What happened and by how much (quantitative)
  - Why it happened (mechanistic interpretation if possible)
  - What it implies for the next experiment or the full run
- **For sweep runs:** Compare against the same-phase baseline, not a different phase's results

## When Logging Sweep Phases

Group runs under a shared subsection (e.g., "Phase 1: LR + Weight Decay") with:
- A table of all runs in the phase
- The best run highlighted
- A combined analysis paragraph drawing conclusions across the group

## Before Running Any Experiment

- State the hypothesis and what variable is being tested
- Pre-register in `training/docs/experiment_registry.md`
- Note the config, HP, and how results will be compared

## After Every Run

- Record results immediately — no "I'll write it up later"
- Update `training/sweep_results.tsv` with raw numbers
- Update `training/docs/experiments.md` with analysis
- Update the summary table — every experiment gets a row
- If the result changes any prior conclusions, update those entries

## Benchmark Logging

Benchmark results (IR quality, end-to-end Gemini) require:
- **Exact command used** to produce the numbers
- **Software state** — git commit hash, binary version, config
- **Environment** — hardware, daemon config, LLM provider and model
- **All metrics** — not just the headline number
- **Comparison context** — what baseline this represents and what it needs to beat

## Do Not

- Write informal one-liner "key findings" — write analysis paragraphs
- Log experiments chronologically instead of by category
- Skip the control/variable line — every experiment is a comparison
- Use vague language ("clearly better", "significantly worse") without numbers
- Cherry-pick metrics — report all of them, including unfavorable ones
- Leave an experiment as "RUNNING" after it finishes
- Run a benchmark, get a number, and not document methodology
