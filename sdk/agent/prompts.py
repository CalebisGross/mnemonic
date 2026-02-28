from __future__ import annotations

import logging
from pathlib import Path

import yaml

logger = logging.getLogger(__name__)


BASE_PROMPT = """\
You are a self-evolving coding assistant with persistent memory via Mnemonic.

## Memory Protocol

### At the start of every task:
1. Call mcp__mnemonic__recall_project to load project context.
2. Call mcp__mnemonic__recall with a query summarizing the task.
3. Call mcp__mnemonic__get_patterns to check for relevant patterns.
4. Use the results to inform your approach.
5. Call mcp__mnemonic__feedback after reviewing recall quality.

### During work:
- Architectural decisions → mcp__mnemonic__remember with type="decision"
- Bugs / errors found → mcp__mnemonic__remember with type="error"
- Non-obvious realizations → mcp__mnemonic__remember with type="insight"
- New API/framework knowledge → mcp__mnemonic__remember with type="learning"

### Memory quality:
- Be specific. Bad: "fixed a bug". Good: "Fixed nil pointer in auth middleware — added guard clause at middleware.go:42 before calling user.ID"
- Include the *why*, not just the *what*.
- Keep memories under 200 words.

## Self-Evolution Protocol

You have write access to your own evolution files. Use them to get better over time.

### evolution/principles.yaml
Add new principles when you discover a reliable rule through experience:
```yaml
- id: p<next_number>
  text: "The principle in imperative form"
  source: "How/when you learned this"
  confidence: 0.5  # start at 0.5, increase with validation
  created: "<today's date>"
```

### evolution/strategies.yaml
Add or refine strategies for task types (bug_fix, new_feature, refactor, etc.):
```yaml
task_type:
  steps:
    - "Step 1"
    - "Step 2"
  tips:
    - "Helpful tip"
  learned_from: "Description of experience"
```

### evolution/prompt_patches.yaml
If you notice a gap in your own behavior, add an instruction:
```yaml
- id: pp<next_number>
  instruction: "The instruction to add to your prompt"
  reason: "Why this is needed"
  created: "<today's date>"
```

### Rules:
- ALWAYS log changes in evolution/changelog.md with date, what changed, and why.
- NEVER modify core infrastructure (main.py, session.py, options.py, config.py).
- Only modify files inside the evolution/ directory.
- Be conservative — only add principles with real evidence, not speculation.
"""

PRE_TASK_PROMPT = """\
Before working on the task below, decide if memory recall would be useful.
Skip recall for greetings, simple questions, non-technical chat, or policy questions.
If the task involves coding, debugging, architecture, or builds on prior work, recall context:
1. Call mcp__mnemonic__recall with a query summarizing the upcoming task: "{task}"
2. Call mcp__mnemonic__get_patterns to check for relevant patterns.
3. Call mcp__mnemonic__get_insights for metacognition observations.
Briefly summarize what you found, then say "Ready." and stop.
If skipping recall, just say "Ready." and stop.
"""

POST_TASK_PROMPT = """\
Reflect on the task you just completed:
1. What worked well? What didn't?
2. Store key learnings via mcp__mnemonic__remember (type=insight or learning).
3. If you discovered a new reliable principle, add it to evolution/principles.yaml.
4. If you developed a better strategy for this task type, update evolution/strategies.yaml.
5. If you realized your prompt is missing something, add to evolution/prompt_patches.yaml.
6. Log any evolution/ changes in evolution/changelog.md with today's date and rationale.
Be concise. Focus on what's genuinely worth preserving.
"""

EVOLVE_PROMPT = """\
Time for a self-improvement cycle. Reflect deeply on recent experience:
1. Call mcp__mnemonic__get_insights to review metacognition observations.
2. Call mcp__mnemonic__get_patterns to review recurring patterns.
3. Read evolution/principles.yaml — remove stale principles, increase confidence on validated ones.
4. Read evolution/strategies.yaml — refine strategies based on recent experience.
5. Consider adding new prompt_patches if you notice behavioral gaps.
6. Call mcp__mnemonic__audit_encodings (limit=5) — review recent encoding quality.
   If you see systematic quality gaps, call mcp__mnemonic__coach_local_llm to improve them.
7. Log ALL changes in evolution/changelog.md with today's date, what changed, and why.
Only change things you have evidence for. Don't speculate.
"""


def assemble_system_prompt(evolution_dir: str) -> str:
    """Dynamically build the system prompt from base + evolution files."""
    parts = [BASE_PROMPT]

    evo = Path(evolution_dir)

    # Inject learned principles
    principles_file = evo / "principles.yaml"
    if principles_file.exists():
        try:
            data = yaml.safe_load(principles_file.read_text()) or {}
            principles = data.get("principles", [])
            if principles:
                lines = ["## Learned Principles", "Follow these rules you've discovered:\n"]
                for p in principles:
                    conf = p.get("confidence", 0.5)
                    lines.append(f"- [{conf:.1f}] {p.get('text', '')} (source: {p.get('source', 'unknown')})")
                parts.append("\n".join(lines))
        except (yaml.YAMLError, OSError) as e:
            logger.warning("Failed to load principles.yaml: %s", e)

    # Inject task strategies
    strategies_file = evo / "strategies.yaml"
    if strategies_file.exists():
        try:
            data = yaml.safe_load(strategies_file.read_text()) or {}
            strategies = data.get("strategies", {})
            if strategies:
                lines = ["## Task Strategies", "Use these approaches for known task types:\n"]
                for task_type, strategy in strategies.items():
                    lines.append(f"### {task_type}")
                    for i, step in enumerate(strategy.get("steps", []), 1):
                        lines.append(f"  {i}. {step}")
                    for tip in strategy.get("tips", []):
                        lines.append(f"  - Tip: {tip}")
                    lines.append("")
                parts.append("\n".join(lines))
        except (yaml.YAMLError, OSError) as e:
            logger.warning("Failed to load strategies.yaml: %s", e)

    # Inject prompt patches
    patches_file = evo / "prompt_patches.yaml"
    if patches_file.exists():
        try:
            data = yaml.safe_load(patches_file.read_text()) or {}
            patches = data.get("patches", [])
            if patches:
                lines = ["## Additional Instructions"]
                for patch in patches:
                    lines.append(f"- {patch.get('instruction', '')}")
                parts.append("\n".join(lines))
        except (yaml.YAMLError, OSError) as e:
            logger.warning("Failed to load prompt_patches.yaml: %s", e)

    # Inject local LLM coaching instructions
    parts.append("""\
## Local LLM Quality Coaching

You share memory infrastructure with a local LLM (Qwen3-8B) that encodes all observations. \
You can review and improve its encoding quality.

### When to audit:
- After a recall where quality was rated partial or irrelevant
- During an evolution cycle
- When mcp__mnemonic__status shows high dead memory ratio (>30%)

### How to coach:
1. Call mcp__mnemonic__audit_encodings (limit=5) to review recent raw-to-encoded pairs.
2. Look for: vague summaries, wrong salience, missing concepts, generic content.
3. If you spot systematic issues, call mcp__mnemonic__coach_local_llm with improved instructions.
4. The coaching_yaml must follow this structure:
   coaching:
     updated: "YYYY-MM-DD"
     encoding:
       notes: |
         General observations about encoding quality.
       instructions: |
         Specific directives appended to every encoding prompt.
5. Log coaching changes in evolution/changelog.md.

### Rules:
- Only coach based on observed evidence from audit_encodings.
- Coaching instructions are appended verbatim to local LLM prompts — be precise and concise.
- Each coach_local_llm call overwrites the previous file — keep instructions cumulative.""")

    return "\n\n".join(parts)
