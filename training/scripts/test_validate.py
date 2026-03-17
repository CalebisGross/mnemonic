#!/usr/bin/env python3
"""Tests for the training data quality gate pipeline."""

import json
import sys
from pathlib import Path

# Add scripts dir to path
sys.path.insert(0, str(Path(__file__).parent))
from validate import validate_encoding, validate_example, ValidationResult


def good_encoding() -> str:
    """Return a valid encoding JSON response."""
    return json.dumps({
        "gist": "User modified auth middleware",
        "summary": "Updated authentication middleware to validate JWT tokens on every request",
        "content": "The auth middleware was updated to check JWT expiry and validate signatures.",
        "narrative": "During a security review, the user identified that the auth middleware was not properly validating JWT tokens. They added expiry checks and signature validation to prevent unauthorized access.",
        "concepts": ["security", "authentication", "api", "fix"],
        "structured_concepts": {
            "topics": [{"label": "auth", "path": "security/auth"}],
            "entities": [{"name": "JWT", "type": "technology", "context": "token validation"}],
            "actions": [{"verb": "updated", "object": "middleware", "details": "added validation"}],
            "causality": [{"relation": "caused_by", "description": "security review identified gap"}],
        },
        "significance": "important",
        "emotional_tone": "satisfying",
        "outcome": "success",
        "salience": 0.7,
    })


def test_valid_encoding():
    result = validate_encoding(good_encoding())
    assert result.valid, f"Expected valid, got failures: {result.hard_failures}"
    assert not result.hard_failures
    print("PASS: test_valid_encoding")


def test_invalid_json():
    result = validate_encoding("not json at all")
    assert not result.valid
    assert "json_parse_failure" in result.hard_failures
    print("PASS: test_invalid_json")


def test_missing_fields():
    result = validate_encoding(json.dumps({"gist": "hello"}))
    assert not result.valid
    assert any("missing_field" in f for f in result.hard_failures)
    print("PASS: test_missing_fields")


def test_gist_too_long():
    data = json.loads(good_encoding())
    data["gist"] = "x" * 61
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert any("gist_too_long" in f for f in result.hard_failures)
    print("PASS: test_gist_too_long")


def test_summary_too_long():
    data = json.loads(good_encoding())
    data["summary"] = "x" * 101
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert any("summary_too_long" in f for f in result.hard_failures)
    print("PASS: test_summary_too_long")


def test_salience_out_of_range():
    data = json.loads(good_encoding())
    data["salience"] = 1.5
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert any("salience_out_of_range" in f for f in result.hard_failures)
    print("PASS: test_salience_out_of_range")


def test_invalid_significance():
    data = json.loads(good_encoding())
    data["significance"] = "super_important"
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert any("invalid_significance" in f for f in result.hard_failures)
    print("PASS: test_invalid_significance")


def test_placeholder_gist():
    data = json.loads(good_encoding())
    data["gist"] = "user did something"
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert "placeholder_gist" in result.hard_failures
    print("PASS: test_placeholder_gist")


def test_empty_content():
    data = json.loads(good_encoding())
    data["content"] = "   "
    result = validate_encoding(json.dumps(data))
    assert not result.valid
    assert "empty_content" in result.hard_failures
    print("PASS: test_empty_content")


def test_soft_warning_low_vocab_coverage():
    data = json.loads(good_encoding())
    data["concepts"] = ["xyzzy", "plugh", "plover", "frobnicate"]
    result = validate_encoding(json.dumps(data))
    assert result.valid  # soft gate, not a hard failure
    assert any("low_vocab_coverage" in w for w in result.soft_warnings)
    print("PASS: test_soft_warning_low_vocab_coverage")


def test_soft_warning_strict_mode():
    data = json.loads(good_encoding())
    data["concepts"] = ["xyzzy", "plugh", "plover", "frobnicate"]
    result = validate_encoding(json.dumps(data), strict=True)
    assert not result.valid  # strict mode rejects soft warnings
    print("PASS: test_soft_warning_strict_mode")


def test_soft_warning_high_salience_routine():
    data = json.loads(good_encoding())
    data["salience"] = 0.95
    data["significance"] = "routine"
    result = validate_encoding(json.dumps(data))
    assert result.valid
    assert "high_salience_routine" in result.soft_warnings
    print("PASS: test_soft_warning_high_salience_routine")


def test_validate_example_with_error():
    example = {"task_type": "encoding", "error": "connection refused", "response": {"content": ""}}
    result = validate_example(example)
    assert not result.valid
    print("PASS: test_validate_example_with_error")


if __name__ == "__main__":
    test_valid_encoding()
    test_invalid_json()
    test_missing_fields()
    test_gist_too_long()
    test_summary_too_long()
    test_salience_out_of_range()
    test_invalid_significance()
    test_placeholder_gist()
    test_empty_content()
    test_soft_warning_low_vocab_coverage()
    test_soft_warning_strict_mode()
    test_soft_warning_high_salience_routine()
    test_validate_example_with_error()
    print(f"\nAll {13} tests passed.")
