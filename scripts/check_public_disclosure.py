#!/usr/bin/env python3
"""Fail if the public repository discloses internal references.

README.md and CHANGELOG.md are checked for every internal/QA disclosure
pattern below. Every other tracked file outside the generated gen/ tree is
checked for internal ticket references (POLY-<number>, in any case), in its
path and in its text, so they cannot come back.
"""

from __future__ import annotations

import re
import subprocess
import sys
from collections.abc import Iterable
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FILES = ("README.md", "CHANGELOG.md")
FORBIDDEN: list[tuple[re.Pattern[str], str]] = [
    (re.compile(r"\bPOLY-\d+\b"), "internal ticket ID"),
    (re.compile(r"\bF-\d+\b"), "internal finding ID"),
    (re.compile(r"\b(?:Yvan|Sergio)\b"), "internal person/reference"),
    (
        re.compile(r"\b(?:current )?(?:devnet|staging) (?:behavior|testing)\b", re.I),
        "environment-specific QA prose",
    ),
    (
        re.compile(r"\b(?:reserve corruption|backend bug|backend issue)\b", re.I),
        "backend incident prose",
    ),
    (re.compile(r"—"), "em dash"),
]

# An internal ticket reference: "POLY", a hyphen and digits, in any case.
TICKET_REFERENCE = re.compile(r"(?<![A-Za-z0-9])poly-\d+(?!\d)", re.IGNORECASE)
# Generated code, copied from the published contract, is not checked.
GENERATED_PREFIXES = ("gen/",)


def tracked_files(root: Path) -> list[str]:
    """Returns the repository's tracked files, relative to root."""
    out = subprocess.run(
        ["git", "-C", str(root), "ls-files", "-z"],
        check=True,
        capture_output=True,
    ).stdout
    return [name for name in out.decode("utf-8").split("\0") if name]


def ticket_references(root: Path, names: Iterable[str]) -> list[str]:
    """Names each file outside gen/ whose path or text holds an internal
    ticket reference, with the first line that does. Files that are not
    UTF-8 text are checked by path only."""
    failures: list[str] = []
    for name in sorted(names):
        if name.startswith(GENERATED_PREFIXES):
            continue
        if TICKET_REFERENCE.search(name):
            failures.append(f"{name}: path contains an internal ticket ID")
            continue
        path = root / name
        if not path.is_file():
            continue
        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        for number, line in enumerate(text.splitlines(), start=1):
            if TICKET_REFERENCE.search(line):
                failures.append(f"{name}:{number}: contains an internal ticket ID")
                break
    return failures


def main() -> int:
    failures: list[str] = []
    for name in FILES:
        path = ROOT / name
        if not path.is_file():
            failures.append(f"{name}: missing")
            continue
        text = path.read_text(encoding="utf-8")
        for pattern, description in FORBIDDEN:
            if pattern.search(text):
                failures.append(f"{name}: contains {description}")
    failures.extend(ticket_references(ROOT, tracked_files(ROOT)))
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("ok: public disclosure checks passed for README.md, CHANGELOG.md and every tracked file outside gen/")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
