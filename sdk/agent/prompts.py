from __future__ import annotations

import logging
from datetime import datetime
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
  last_reinforced: "<today's date>"  # reset when confidence increases
```
Note: Principles decay automatically (~2%/day after 14 days without reinforcement).
When you validate a principle, update both `confidence` and `last_reinforced`.

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
   When you increase a principle's confidence, also set `last_reinforced: "<today's date>"` (YYYY-MM-DD).
   This resets the automatic decay timer. Principles that go unreinforced for 14+ days lose confidence
   at ~2%/day and are pruned below 0.3 — so only reinforce principles you have fresh evidence for.
4. Read evolution/strategies.yaml — refine strategies based on recent experience.
5. Consider adding new prompt_patches if you notice behavioral gaps.
6. Call mcp__mnemonic__audit_encodings (limit=5) — review recent encoding quality.
   If you see systematic quality gaps, call mcp__mnemonic__coach_local_llm to improve them.
7. Log ALL changes in evolution/changelog.md with today's date, what changed, and why.
Only change things you have evidence for. Don't speculate.
"""


_PRINCIPLES_MIN_CONFIDENCE = 0.6
_PRINCIPLES_MAX = 15
_STRATEGIES_INCLUDE_TIPS = False
_PATCHES_MAX = 20
_PROMPT_BUDGET = 20_000

_DECAY_GRACE_DAYS = 14
_DECAY_RATE_PER_DAY = 0.98
_DECAY_PRUNE_THRESHOLD = 0.3


def _decay_stale_principles(evolution_dir: str) -> None:
    """Apply time-based confidence decay to principles that haven't been reinforced.

    - 14-day grace period from created/last_reinforced date.
    - After grace: multiply confidence by 0.98 per elapsed day.
    - Below 0.3 confidence: prune from the file.
    - Runs at most once per day (stamp file).
    """
    evo = Path(evolution_dir)
    principles_file = evo / "principles.yaml"
    if not principles_file.exists():
        return

    # Rate-limit: only run decay once per day
    stamp_file = evo / ".decay_stamp"
    today = datetime.now().strftime("%Y-%m-%d")
    if stamp_file.exists():
        try:
            if stamp_file.read_text().strip() == today:
                return
        except OSError:
            pass

    try:
        data = yaml.safe_load(principles_file.read_text()) or {}
    except (yaml.YAMLError, OSError) as e:
        logger.warning("Failed to load principles.yaml for decay: %s", e)
        return

    principles = data.get("principles", [])
    if not principles:
        return

    now = datetime.now()
    changed = False
    surviving = []

    for p in principles:
        # Determine the anchor date (last_reinforced takes priority over created)
        anchor_str = p.get("last_reinforced") or p.get("created")
        if not anchor_str:
            surviving.append(p)
            continue

        try:
            anchor = datetime.strptime(str(anchor_str), "%Y-%m-%d")
        except ValueError:
            surviving.append(p)
            continue

        days_since = (now - anchor).days
        if days_since <= _DECAY_GRACE_DAYS:
            surviving.append(p)
            continue

        # Apply multiplicative decay for days beyond grace period
        decay_days = days_since - _DECAY_GRACE_DAYS
        confidence = p.get("confidence", 0.5)
        new_confidence = confidence * (_DECAY_RATE_PER_DAY ** decay_days)
        new_confidence = round(new_confidence, 3)

        if new_confidence < _DECAY_PRUNE_THRESHOLD:
            logger.info("Pruning principle %s (confidence %.3f < %.1f)",
                        p.get("id", "?"), new_confidence, _DECAY_PRUNE_THRESHOLD)
            changed = True
            continue  # skip — don't add to surviving

        if new_confidence != confidence:
            p["confidence"] = new_confidence
            changed = True

        surviving.append(p)

    if changed:
        data["principles"] = surviving
        try:
            principles_file.write_text(yaml.dump(data, default_flow_style=False, sort_keys=False))
            logger.info("Decayed principles: %d kept, %d pruned",
                        len(surviving), len(principles) - len(surviving))
        except OSError as e:
            logger.warning("Failed to write decayed principles: %s", e)

    # Write stamp regardless of whether changes occurred — decay was evaluated today
    try:
        stamp_file.write_text(today)
    except OSError:
        pass


def assemble_system_prompt(evolution_dir: str) -> str:
    """Dynamically build the system prompt from base + evolution files."""
    # Decay stale principles before building the prompt
    _decay_stale_principles(evolution_dir)

    parts = [BASE_PROMPT]

    evo = Path(evolution_dir).resolve()

    # Tell the agent exactly where its evolution files live
    parts.append(f"## Evolution Directory\nYour evolution files are at: `{evo}/`\nWhen reading or writing evolution files, use this absolute path.")

    # Inject learned principles (filtered by confidence, capped)
    principles_file = evo / "principles.yaml"
    if principles_file.exists():
        try:
            data = yaml.safe_load(principles_file.read_text()) or {}
            principles = data.get("principles", [])
            # Filter low-confidence, sort by confidence desc, cap
            principles = [
                p for p in principles
                if p.get("confidence", 0.0) >= _PRINCIPLES_MIN_CONFIDENCE
            ]
            principles.sort(key=lambda p: p.get("confidence", 0.0), reverse=True)
            principles = principles[:_PRINCIPLES_MAX]
            if principles:
                lines = ["## Learned Principles", "Follow these rules you've discovered:\n"]
                for p in principles:
                    conf = p.get("confidence", 0.5)
                    lines.append(f"- [{conf:.1f}] {p.get('text', '')} (source: {p.get('source', 'unknown')})")
                parts.append("\n".join(lines))
        except (yaml.YAMLError, OSError) as e:
            logger.warning("Failed to load principles.yaml: %s", e)

    # Inject task strategies (steps only — tips omitted to save budget)
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
                    if _STRATEGIES_INCLUDE_TIPS:
                        for tip in strategy.get("tips", []):
                            lines.append(f"  - Tip: {tip}")
                    lines.append("")
                parts.append("\n".join(lines))
        except (yaml.YAMLError, OSError) as e:
            logger.warning("Failed to load strategies.yaml: %s", e)

    # Inject prompt patches (capped)
    patches_file = evo / "prompt_patches.yaml"
    if patches_file.exists():
        try:
            data = yaml.safe_load(patches_file.read_text()) or {}
            patches = (data.get("patches") or data.get("prompt_patches") or [])[:_PATCHES_MAX]
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

    prompt = "\n\n".join(parts)
    if len(prompt) > _PROMPT_BUDGET:
        logger.warning(
            "System prompt exceeds budget: %d chars (limit %d)",
            len(prompt), _PROMPT_BUDGET,
        )
    return prompt
