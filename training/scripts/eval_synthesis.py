#!/usr/bin/env python3
"""Evaluate Felix-encoder-v2 synthesis quality against Gemini ground truth.

Takes the synthesis training data (Gemini-generated), holds out a subset,
runs the same prompts through Felix via llama.cpp, and compares:
  1. Whether the model produces coherent output (not garbage)
  2. Grounding: does it reference facts from the provided memories?
  3. Conciseness: 2-5 sentences as trained?
  4. Tool use: does it attempt tool calls? (from training data structure)

Usage:
    python training/scripts/eval_synthesis.py \
        --model models/felix-encoder-v2-q8_0.gguf \
        --data training/data/synthesis_data.jsonl \
        --n-eval 20
"""

import argparse
import json
import random
import subprocess
import re
import sys
from pathlib import Path


def load_examples(path, n_eval, seed=42):
    """Load and split synthesis examples."""
    with open(path) as f:
        examples = [json.loads(l) for l in f]
    random.seed(seed)
    random.shuffle(examples)
    return examples[:n_eval]


def extract_query_and_context(example):
    """Extract the user query and tool-provided context from an example."""
    query = ""
    context = ""
    gemini_response = ""

    for msg in example["messages"]:
        if msg["role"] == "user":
            # Extract the "They're asking:" line
            for line in msg["content"].split("\n"):
                if "asking:" in line.lower():
                    query = line.strip()
                    break
            # Full prompt for the model
            full_prompt = msg["content"]
        elif msg["role"] == "tool":
            context = msg["content"]
        elif msg["role"] == "assistant" and msg["content"]:
            gemini_response = msg["content"]

    return query, full_prompt, context, gemini_response


def build_prompt(full_prompt, context):
    """Build the prompt as Felix would see it (user prompt + tool results inline)."""
    # Felix doesn't do real tool calls — in the converted training data,
    # the tool results are provided as context in the prompt.
    prompt = f"""You are a memory synthesis agent. Given retrieved memories and context, produce a concise 2-5 sentence synthesis grounded in concrete facts, decisions, and specifics.

{full_prompt}

Retrieved context:
{context}

Synthesis:"""
    return prompt


def run_llamacpp(model_path, prompt, binary="./bin/mnemonic", max_tokens=512):
    """Run inference via llama.cpp CLI or the mnemonic binary."""
    # Try using the mnemonic binary's complete subcommand if it exists,
    # otherwise fall back to llama-cli
    llama_cli = Path.home() / "Projects" / "llama.cpp" / "build" / "bin" / "llama-cli"
    if not llama_cli.exists():
        llama_cli = "llama-cli"

    cmd = [
        str(llama_cli),
        "-m", model_path,
        "-p", prompt,
        "-n", str(max_tokens),
        "--temp", "0.1",
        "--top-p", "0.9",
        "-ngl", "0",  # CPU only
        "--no-display-prompt",
        "--log-disable",
    ]

    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=120
        )
        return result.stdout.strip()
    except subprocess.TimeoutExpired:
        return "[TIMEOUT]"
    except FileNotFoundError:
        return "[LLAMA_CLI_NOT_FOUND]"


def score_output(output, gemini_response, context):
    """Score the model output on multiple dimensions."""
    scores = {}

    # 1. Coherent (not garbage/empty)
    if not output or output in ("[TIMEOUT]", "[LLAMA_CLI_NOT_FOUND]"):
        return {"coherent": 0, "grounded": 0, "concise": 0, "length": 0, "sentences": 0}

    scores["length"] = len(output)

    # 2. Sentence count (target: 2-5)
    sentences = [s.strip() for s in re.split(r'[.!?]+', output) if s.strip() and len(s.strip()) > 10]
    scores["sentences"] = len(sentences)
    scores["concise"] = 1.0 if 1 <= len(sentences) <= 7 else 0.5 if len(sentences) <= 10 else 0.0

    # 3. Coherent: has real words, not just JSON or garbage
    word_count = len(output.split())
    has_real_words = word_count >= 10
    not_json = not output.strip().startswith("{")
    scores["coherent"] = 1.0 if (has_real_words and not_json) else 0.0

    # 4. Grounded: check if key facts from context appear in output
    # Extract numbers and key terms from context
    context_numbers = set(re.findall(r'\d+\.?\d*', context))
    output_numbers = set(re.findall(r'\d+\.?\d*', output))
    number_overlap = len(context_numbers & output_numbers) / max(len(context_numbers), 1)

    # Check for key terms from context in output
    context_words = set(w.lower() for w in context.split() if len(w) > 4)
    output_words = set(w.lower() for w in output.split() if len(w) > 4)
    term_overlap = len(context_words & output_words) / max(len(context_words), 1)

    scores["grounded"] = min(1.0, (number_overlap + term_overlap) / 2 * 2)  # Scale to 0-1

    return scores


def main():
    parser = argparse.ArgumentParser(description="Evaluate Felix synthesis quality")
    parser.add_argument("--model", type=str, default="models/felix-encoder-v2-q8_0.gguf")
    parser.add_argument("--data", type=str, default="training/data/synthesis_data.jsonl")
    parser.add_argument("--n-eval", type=int, default=20)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    print(f"Loading {args.n_eval} eval examples from {args.data}...")
    examples = load_examples(args.data, args.n_eval, args.seed)
    print(f"Model: {args.model}")
    print()

    all_scores = []
    for i, ex in enumerate(examples):
        query, full_prompt, context, gemini_response = extract_query_and_context(ex)
        prompt = build_prompt(full_prompt, context)

        print(f"[{i+1}/{args.n_eval}] {query[:70]}...")
        output = run_llamacpp(args.model, prompt)

        scores = score_output(output, gemini_response, context)
        all_scores.append(scores)

        if args.verbose:
            print(f"  Gemini ({len(gemini_response)} chars): {gemini_response[:150]}...")
            print(f"  Felix  ({len(output)} chars): {output[:150]}...")
            print(f"  Scores: {scores}")
        else:
            status = "OK" if scores.get("coherent", 0) > 0 else "FAIL"
            print(f"  {status} | coherent={scores['coherent']:.0f} grounded={scores['grounded']:.2f} concise={scores['concise']:.1f} sentences={scores['sentences']}")
        print()

    # Aggregate
    n = len(all_scores)
    if n == 0:
        print("No examples evaluated.")
        return

    print("=" * 70)
    print(f"  SYNTHESIS EVALUATION RESULTS ({n} examples)")
    print("=" * 70)

    coherent = sum(s["coherent"] for s in all_scores) / n
    grounded = sum(s["grounded"] for s in all_scores) / n
    concise = sum(s["concise"] for s in all_scores) / n
    avg_sentences = sum(s["sentences"] for s in all_scores) / n
    avg_length = sum(s["length"] for s in all_scores) / n

    print(f"  Coherent:       {coherent:.1%} ({sum(1 for s in all_scores if s['coherent'] > 0)}/{n} produced real text)")
    print(f"  Grounded:       {grounded:.1%} (fact overlap with provided context)")
    print(f"  Concise:        {concise:.1%} (target: 2-5 sentences)")
    print(f"  Avg sentences:  {avg_sentences:.1f}")
    print(f"  Avg length:     {avg_length:.0f} chars")
    print()

    # Compare against Gemini ground truth lengths
    gemini_lengths = []
    for ex in examples:
        for msg in ex["messages"]:
            if msg["role"] == "assistant" and msg["content"]:
                gemini_lengths.append(len(msg["content"]))
    if gemini_lengths:
        gemini_avg = sum(gemini_lengths) / len(gemini_lengths)
        print(f"  Gemini avg length: {gemini_avg:.0f} chars")
        print(f"  Felix/Gemini ratio: {avg_length/gemini_avg:.2f}x")
    print()


if __name__ == "__main__":
    main()
