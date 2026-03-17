#!/usr/bin/env python3
"""Unified text cleaning for pretraining data.

Each source type has specific cleaning needs, but all share common
normalization steps. Use clean_document() for the general pipeline
or source-specific functions for targeted cleaning.
"""

import re
import unicodedata


def normalize_text(text: str) -> str:
    """Common normalization applied to all sources."""
    # Unicode normalization (NFKC)
    text = unicodedata.normalize("NFKC", text)
    # Normalize line endings
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    # Collapse 3+ consecutive newlines to 2
    text = re.sub(r"\n{3,}", "\n\n", text)
    # Strip leading/trailing whitespace
    text = text.strip()
    return text


def clean_academic_paper(text: str) -> str:
    """Clean an academic paper (PeS2o / arXiv)."""
    text = normalize_text(text)
    # Strip common LaTeX commands (keep the content inside)
    text = re.sub(r"\\(?:textbf|textit|emph|underline)\{([^}]*)\}", r"\1", text)
    text = re.sub(r"\\(?:cite|ref|label|eqref)\{[^}]*\}", "", text)
    text = re.sub(r"\\(?:begin|end)\{[^}]*\}", "", text)
    # Remove inline math delimiters but keep content
    text = re.sub(r"\$([^$]+)\$", r"\1", text)
    # Remove figure/table references like [Figure 1] or (Table 2)
    text = re.sub(r"[\[(](?:Figure|Fig\.|Table|Tab\.)\s*\d+[)\]]", "", text, flags=re.IGNORECASE)
    # Remove reference section (heuristic: after "References" heading near end)
    ref_pattern = re.compile(r"\n\s*References\s*\n", re.IGNORECASE)
    match = ref_pattern.search(text, pos=len(text) // 2)  # only look in second half
    if match:
        text = text[:match.start()]
    return text.strip()


def clean_code(text: str) -> str:
    """Clean source code."""
    text = normalize_text(text)
    # Normalize trailing whitespace per line
    text = "\n".join(line.rstrip() for line in text.split("\n"))
    return text


def clean_web_text(text: str) -> str:
    """Clean web text (FineWeb-Edu)."""
    text = normalize_text(text)
    # FineWeb-Edu is already well-cleaned; just normalize
    return text


def clean_stackoverflow(text: str) -> str:
    """Clean StackOverflow Q&A (HTML stripped before this point)."""
    text = normalize_text(text)
    # Normalize code block indentation
    text = re.sub(r"<code>(.*?)</code>", r"`\1`", text, flags=re.DOTALL)
    # Strip any remaining HTML tags
    text = re.sub(r"<[^>]+>", "", text)
    # Decode common HTML entities
    text = text.replace("&amp;", "&").replace("&lt;", "<").replace("&gt;", ">")
    text = text.replace("&quot;", '"').replace("&#39;", "'")
    return text


def clean_json(text: str) -> str:
    """Clean JSON/structured data."""
    text = normalize_text(text)
    return text


def clean_commit(text: str) -> str:
    """Clean git commit message + diff."""
    text = normalize_text(text)
    # Cap diff size to prevent huge diffs from dominating
    lines = text.split("\n")
    if len(lines) > 100:
        text = "\n".join(lines[:100]) + "\n[diff truncated]"
    return text


# Source type -> cleaner function mapping
CLEANERS = {
    "pes2o_neuro": clean_academic_paper,
    "pes2o_cs": clean_academic_paper,
    "code": clean_code,
    "fineweb": clean_web_text,
    "stackoverflow": clean_stackoverflow,
    "json_structured": clean_json,
    "commits": clean_commit,
}


def clean_document(text: str, source: str) -> str | None:
    """Clean a document based on its source type.

    Returns None if the document should be filtered out.
    """
    cleaner = CLEANERS.get(source, normalize_text)
    text = cleaner(text)

    # Universal quality filter
    if len(text) < 100:
        return None

    return text
