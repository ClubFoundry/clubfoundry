#!/usr/bin/env python3
"""Validate the sidecar runtime inventory and bundled release evidence."""

from __future__ import annotations

import hashlib
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent
RUNTIME_LOCK = ROOT / "runtime-packages.lock"
RUNTIME_TOOLS_LOCK = ROOT / "runtime-tools.lock"
TEXT_LOCK = ROOT / "LICENSES" / "license-texts.lock"
LICENSE_TOKEN = re.compile(r"[A-Za-z][A-Za-z0-9.+-]*")
OPERATORS = {"AND", "OR", "WITH"}
SPECIAL_LICENSES = {"Public-Domain"}
ADDITIONAL_LICENSES = {"BSD-3-Clause"}  # Go standard library.


def fail(message: str) -> None:
    raise ValueError(message)


def parse_runtime_lock() -> set[str]:
    packages: list[str] = []
    licenses: set[str] = set()
    raw_lines = RUNTIME_LOCK.read_text(encoding="utf-8").splitlines()
    for line_number, raw_line in enumerate(raw_lines, 1):
        parts = raw_line.split("|")
        if len(parts) != 3 or not all(parts):
            fail(f"invalid runtime lock entry at line {line_number}")
        package, _version, expression = parts
        if package in packages:
            fail(f"duplicate runtime package: {package}")
        packages.append(package)
        tokens = set(LICENSE_TOKEN.findall(expression)) - OPERATORS
        if not tokens:
            fail(f"missing license expression for {package}")
        licenses.update(tokens)
    if raw_lines != sorted(raw_lines):
        fail("runtime package lock must be sorted by complete lock entry")
    return licenses | ADDITIONAL_LICENSES


def parse_runtime_tools_lock() -> set[str]:
    raw_lines = RUNTIME_TOOLS_LOCK.read_text(encoding="utf-8").splitlines()
    expected = {
        "docker-cli": (
            "v29.5.3",
            "sha256:11e1133c30f3ceb73c6bdc7dfb78b3f9"
            "ed8e8e0d1d0400e91c5ec2eb240bf2ff",
            "docker:29.5.3-cli-alpine3.24",
            "docker_cli",
        ),
        "docker-compose": (
            "v5.4.0",
            "sha256:b03e46987ca4ebb41ca31b765ad7ba957"
            "388003f1ef1255fd68333a7b838d632",
            "docker/compose-bin:v5.4.0",
            "compose",
        ),
    }
    if len(raw_lines) != len(expected) or raw_lines != sorted(raw_lines):
        fail("runtime tool lock must contain the sorted reviewed entries")

    licenses: set[str] = set()
    dockerfile = (ROOT / "Dockerfile").read_text(encoding="utf-8")
    seen: set[str] = set()
    for line_number, raw_line in enumerate(raw_lines, 1):
        parts = raw_line.split("|")
        if len(parts) != 4:
            fail(f"invalid runtime tool lock entry at line {line_number}")
        tool, version, digest, expression = parts
        if tool in seen or tool not in expected:
            fail(f"unexpected or duplicate runtime tool: {tool}")
        seen.add(tool)
        expected_version, expected_digest, image, stage = expected[tool]
        if version != expected_version or digest != expected_digest:
            fail(f"unexpected {tool} version or image digest")
        expected_source = f"FROM {image}@{digest} AS {stage}"
        if expected_source not in dockerfile:
            fail(f"Dockerfile does not use the locked {tool} image")
        tokens = set(LICENSE_TOKEN.findall(expression)) - OPERATORS
        if not tokens:
            fail(f"missing runtime tool license expression for {tool}")
        licenses.update(tokens)
    return licenses


def parse_text_lock() -> dict[str, tuple[str, str]]:
    entries: dict[str, tuple[str, str]] = {}
    for line_number, raw_line in enumerate(
        TEXT_LOCK.read_text(encoding="utf-8").splitlines(), 1
    ):
        parts = raw_line.split("|")
        if len(parts) != 3:
            fail(f"invalid license text lock entry at line {line_number}")
        identifier, digest, source = parts
        if identifier in entries:
            fail(f"duplicate license text: {identifier}")
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            fail(f"invalid SHA-256 for {identifier}")
        expected_source = (
            "https://raw.githubusercontent.com/spdx/license-list-data/"
            f"v3.28.0/text/{identifier}.txt"
        )
        if source != expected_source:
            fail(f"unpinned or unexpected source for {identifier}")
        entries[identifier] = (digest, source)
    return entries


def validate() -> None:
    required = parse_runtime_lock() | parse_runtime_tools_lock()
    text_entries = parse_text_lock()
    expected_texts = required - SPECIAL_LICENSES
    if set(text_entries) != expected_texts:
        missing = sorted(expected_texts - set(text_entries))
        unexpected = sorted(set(text_entries) - expected_texts)
        fail(f"license text set mismatch; missing={missing}, unexpected={unexpected}")

    for identifier, (expected_digest, _source) in text_entries.items():
        path = ROOT / "LICENSES" / f"{identifier}.txt"
        if not path.is_file() or path.stat().st_size == 0:
            fail(f"missing license text: {identifier}")
        actual_digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if actual_digest != expected_digest:
            fail(f"license text checksum mismatch: {identifier}")

    required_files = (
        ROOT / "LICENSES" / "README.md",
        ROOT / "LICENSES" / "Public-Domain.txt",
        ROOT / "THIRD_PARTY_NOTICES.md",
        ROOT / "SOURCE_OFFER.md",
    )
    for path in required_files:
        if not path.is_file() or path.stat().st_size == 0:
            fail(f"missing release evidence file: {path.relative_to(ROOT)}")


def main() -> int:
    try:
        validate()
    except (OSError, UnicodeError, ValueError) as error:
        print(f"sidecar release evidence failed: {error}", file=sys.stderr)
        return 1
    print("sidecar release evidence: runtime licenses and pinned texts verified")
    return 0


if __name__ == "__main__":
    sys.exit(main())
