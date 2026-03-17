#!/usr/bin/env python3
"""
Quality gate pipeline for mnemonic training data.

Reads raw JSONL captures from the training data directory, applies hard and soft
quality gates, and writes validated examples to the validated/ directory.
Rejected examples go to rejected/ with rejection reasons.

Usage:
    python validate.py [--input-dir DIR] [--output-dir DIR] [--strict]

Hard gates (auto-reject):
    - JSON parse failure
    - Missing required fields
    - Field constraint violations (gist >60 chars, summary >100 chars, etc.)
    - Salience outside [0, 1]
    - Invalid significance/emotional_tone/outcome enum values
    - Empty or placeholder content

Soft gates (flagged for review, pass by default):
    - Low concept vocabulary coverage
    - Suspiciously short narrative
    - Salience outliers (> 0.9 or < 0.1 for non-trivial content)
    With --strict, soft gate failures also reject.
"""

import argparse
import json
import os
import sys
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path

# Controlled vocabulary from config.example.yaml
CONTROLLED_VOCABULARY = {
    "go", "python", "javascript", "typescript", "sql", "bash", "html", "css",
    "docker", "git", "linux", "macos", "systemd", "build", "ci", "deployment",
    "debugging", "testing", "refactoring", "configuration", "migration",
    "documentation", "review", "api", "database", "filesystem", "networking",
    "security", "authentication", "performance", "logging", "ui", "cli",
    "memory", "encoding", "retrieval", "embedding", "agent", "llm", "daemon",
    "mcp", "watcher", "decision", "error", "fix", "insight", "learning",
    "planning", "research", "dependency", "schema", "config",
}

VALID_SIGNIFICANCE = {"routine", "notable", "important", "critical"}
VALID_EMOTIONAL_TONE = {"neutral", "satisfying", "frustrating", "exciting", "concerning"}
VALID_OUTCOME = {"success", "failure", "ongoing", "unknown"}

PLACEHOLDER_GISTS = {
    "user did something",
    "something happened",
    "file changed",
    "event occurred",
    "unknown event",
    "observation",
}


@dataclass
class ValidationResult:
    valid: bool = True
    hard_failures: list = field(default_factory=list)
    soft_warnings: list = field(default_factory=list)


def validate_encoding(response_content: str, strict: bool = False) -> ValidationResult:
    """Validate an encoding task response against quality gates."""
    result = ValidationResult()

    # Hard gate: JSON parse
    try:
        data = json.loads(response_content)
    except (json.JSONDecodeError, TypeError):
        result.valid = False
        result.hard_failures.append("json_parse_failure")
        return result

    if not isinstance(data, dict):
        result.valid = False
        result.hard_failures.append("response_not_object")
        return result

    # Hard gate: required fields
    required = [
        "gist", "summary", "content", "narrative", "concepts",
        "structured_concepts", "significance", "emotional_tone",
        "outcome", "salience",
    ]
    for f in required:
        if f not in data:
            result.valid = False
            result.hard_failures.append(f"missing_field:{f}")

    if not result.valid:
        return result

    # Hard gate: field types
    if not isinstance(data.get("gist"), str):
        result.valid = False
        result.hard_failures.append("gist_not_string")
    if not isinstance(data.get("summary"), str):
        result.valid = False
        result.hard_failures.append("summary_not_string")
    if not isinstance(data.get("concepts"), list):
        result.valid = False
        result.hard_failures.append("concepts_not_array")
    if not isinstance(data.get("salience"), (int, float)):
        result.valid = False
        result.hard_failures.append("salience_not_number")

    if not result.valid:
        return result

    # Hard gate: field constraints
    gist = data["gist"]
    summary = data["summary"]
    content = data.get("content", "")
    narrative = data.get("narrative", "")
    salience = data["salience"]
    significance = data.get("significance", "")
    emotional_tone = data.get("emotional_tone", "")
    outcome = data.get("outcome", "")
    concepts = data.get("concepts", [])

    if len(gist) > 60:
        result.valid = False
        result.hard_failures.append(f"gist_too_long:{len(gist)}")

    if len(summary) > 100:
        result.valid = False
        result.hard_failures.append(f"summary_too_long:{len(summary)}")

    if not (0.0 <= salience <= 1.0):
        result.valid = False
        result.hard_failures.append(f"salience_out_of_range:{salience}")

    if significance and significance not in VALID_SIGNIFICANCE:
        result.valid = False
        result.hard_failures.append(f"invalid_significance:{significance}")

    if emotional_tone and emotional_tone not in VALID_EMOTIONAL_TONE:
        result.valid = False
        result.hard_failures.append(f"invalid_emotional_tone:{emotional_tone}")

    if outcome and outcome not in VALID_OUTCOME:
        result.valid = False
        result.hard_failures.append(f"invalid_outcome:{outcome}")

    # Hard gate: placeholder content
    if gist.lower().strip() in PLACEHOLDER_GISTS:
        result.valid = False
        result.hard_failures.append("placeholder_gist")

    if not content.strip():
        result.valid = False
        result.hard_failures.append("empty_content")

    if not result.valid:
        return result

    # Soft gate: concept vocabulary coverage
    if concepts:
        vocab_hits = sum(1 for c in concepts if c.lower() in CONTROLLED_VOCABULARY)
        coverage = vocab_hits / len(concepts) if concepts else 0
        if coverage < 0.3:
            result.soft_warnings.append(f"low_vocab_coverage:{coverage:.2f}")

    # Soft gate: short narrative
    if len(narrative) < 20:
        result.soft_warnings.append(f"short_narrative:{len(narrative)}")

    # Soft gate: salience outliers
    if salience > 0.9 and significance == "routine":
        result.soft_warnings.append("high_salience_routine")
    if salience < 0.1 and significance in ("important", "critical"):
        result.soft_warnings.append("low_salience_important")

    # Soft gate: few concepts for substantive content
    if len(content) > 200 and len(concepts) < 3:
        result.soft_warnings.append(f"few_concepts:{len(concepts)}")

    if strict and result.soft_warnings:
        result.valid = False

    return result


def validate_example(example: dict, strict: bool = False) -> ValidationResult:
    """Validate a single captured training example."""
    task_type = example.get("task_type", "unknown")

    # Only validate encoding tasks for now (the primary training target)
    if task_type == "encoding":
        response_content = example.get("response", {}).get("content", "")
        return validate_encoding(response_content, strict=strict)

    # For non-encoding tasks, just check basic structure
    result = ValidationResult()
    if example.get("error"):
        result.valid = False
        result.hard_failures.append("call_error")
    if not example.get("parse_success", True):
        result.valid = False
        result.hard_failures.append("parse_failure")
    return result


def process_file(
    input_path: Path,
    validated_dir: Path,
    rejected_dir: Path,
    strict: bool = False,
) -> dict:
    """Process a single JSONL capture file through quality gates."""
    stats = Counter()

    validated_path = validated_dir / input_path.name
    rejected_path = rejected_dir / input_path.name

    with (
        open(input_path) as fin,
        open(validated_path, "a") as fval,
        open(rejected_path, "a") as frej,
    ):
        for line_num, line in enumerate(fin, 1):
            line = line.strip()
            if not line:
                continue

            try:
                example = json.loads(line)
            except json.JSONDecodeError:
                stats["parse_error"] += 1
                continue

            task_type = example.get("task_type", "unknown")
            stats[f"total_{task_type}"] += 1

            result = validate_example(example, strict=strict)

            if result.valid:
                stats[f"valid_{task_type}"] += 1
                # Add validation metadata
                example["_validation"] = {
                    "warnings": result.soft_warnings,
                    "validated_at": None,  # Will be set by downstream
                }
                fval.write(json.dumps(example) + "\n")
            else:
                stats[f"rejected_{task_type}"] += 1
                example["_rejection"] = {
                    "hard_failures": result.hard_failures,
                    "soft_warnings": result.soft_warnings,
                    "source_file": str(input_path),
                    "source_line": line_num,
                }
                frej.write(json.dumps(example) + "\n")

            if result.soft_warnings:
                stats["soft_warnings"] += len(result.soft_warnings)
                for w in result.soft_warnings:
                    stats[f"warning_{w.split(':')[0]}"] += 1

            for f in result.hard_failures:
                stats[f"failure_{f.split(':')[0]}"] += 1

    return dict(stats)


def main():
    parser = argparse.ArgumentParser(description="Validate mnemonic training data")
    parser.add_argument(
        "--input-dir",
        default=os.path.expanduser("~/.mnemonic/training-data"),
        help="Directory containing raw JSONL captures",
    )
    parser.add_argument(
        "--output-dir",
        default="data",
        help="Base output directory (validated/ and rejected/ subdirs)",
    )
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Reject examples that fail soft gates too",
    )
    args = parser.parse_args()

    input_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)
    validated_dir = output_dir / "validated"
    rejected_dir = output_dir / "rejected"

    validated_dir.mkdir(parents=True, exist_ok=True)
    rejected_dir.mkdir(parents=True, exist_ok=True)

    if not input_dir.exists():
        print(f"Input directory does not exist: {input_dir}")
        print("Enable training data capture in config.yaml first.")
        sys.exit(1)

    jsonl_files = sorted(input_dir.glob("capture_*.jsonl"))
    if not jsonl_files:
        print(f"No capture files found in {input_dir}")
        sys.exit(1)

    total_stats = Counter()
    for fpath in jsonl_files:
        print(f"Processing {fpath.name}...")
        stats = process_file(fpath, validated_dir, rejected_dir, strict=args.strict)
        total_stats.update(stats)

    # Print summary
    print("\n--- Validation Summary ---")
    total = sum(v for k, v in total_stats.items() if k.startswith("total_"))
    valid = sum(v for k, v in total_stats.items() if k.startswith("valid_"))
    rejected = sum(v for k, v in total_stats.items() if k.startswith("rejected_"))

    print(f"Total examples:    {total}")
    print(f"Validated:         {valid} ({valid/total*100:.1f}%)" if total else "")
    print(f"Rejected:          {rejected} ({rejected/total*100:.1f}%)" if total else "")

    if total_stats.get("soft_warnings"):
        print(f"Soft warnings:     {total_stats['soft_warnings']}")

    print("\nBy task type:")
    for key in sorted(total_stats):
        if key.startswith("total_"):
            task = key.replace("total_", "")
            v = total_stats.get(f"valid_{task}", 0)
            r = total_stats.get(f"rejected_{task}", 0)
            t = total_stats[key]
            print(f"  {task}: {t} total, {v} valid, {r} rejected")

    if any(k.startswith("failure_") for k in total_stats):
        print("\nRejection reasons:")
        for key in sorted(total_stats):
            if key.startswith("failure_"):
                reason = key.replace("failure_", "")
                print(f"  {reason}: {total_stats[key]}")

    if any(k.startswith("warning_") for k in total_stats):
        print("\nWarning types:")
        for key in sorted(total_stats):
            if key.startswith("warning_"):
                reason = key.replace("warning_", "")
                print(f"  {reason}: {total_stats[key]}")

    print(f"\nValidated data written to: {validated_dir}")
    print(f"Rejected data written to:  {rejected_dir}")


if __name__ == "__main__":
    main()
