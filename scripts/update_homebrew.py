#!/usr/bin/env python3
"""Verify a published Namu release and render its Homebrew formula."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

from release import TARGETS, archive_names, validate_tag

REPOSITORY = "kamaal111/namu"
TEMPLATE = Path(__file__).resolve().parents[1] / "packaging/homebrew/namu.rb.in"


def render_formula(tag: str, assets: Path, template: str) -> str:
    version = validate_tag(tag)
    expected = set(archive_names(tag))
    checksums = {}
    for line in (assets / "checksums.txt").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  (namu_[a-zA-Z0-9_.-]+\.tar\.gz)", line)
        if not match or match[2] in checksums:
            raise ValueError("Malformed or duplicate release checksum")
        checksums[match[2]] = match[1]
    if set(checksums) != expected:
        raise ValueError("Checksums must describe exactly the four supported release archives")
    formula = template.replace("@VERSION@", version)
    for target in TARGETS:
        name = f"namu_{version}_{target}.tar.gz"
        digest = hashlib.sha256((assets / name).read_bytes()).hexdigest()
        if digest != checksums[name]:
            raise ValueError(f"Checksum mismatch: {name}")
        formula = formula.replace(f"@SHA_{target.upper()}@", digest)
    if re.search(r"@[A-Z_]+@", formula):
        raise ValueError("Unresolved formula placeholder")
    return formula


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", help="Stable vX.Y.Z tag; defaults to latest stable release")
    parser.add_argument("--output", type=Path, required=True, help="Destination formula path")
    args = parser.parse_args()
    try:
        if args.version:
            validate_tag(args.version)
        command = ["gh", "release", "view"]
        if args.version:
            command.append(args.version)
        metadata = json.loads(subprocess.check_output(
            command + ["--repo", REPOSITORY, "--json", "tagName,isDraft,isPrerelease,assets"], text=True,
        ))
        tag = metadata["tagName"]
        validate_tag(tag)
        if metadata["isDraft"] or metadata["isPrerelease"]:
            raise ValueError("Expected a published stable release")
        expected = {"checksums.txt"} | set(archive_names(tag))
        if not expected.issubset({asset["name"] for asset in metadata["assets"]}):
            raise ValueError("Release assets are incomplete")
        with tempfile.TemporaryDirectory(prefix="namu-homebrew-") as temp:
            for name in sorted(expected):
                subprocess.run(["gh", "release", "download", tag, "--repo", REPOSITORY,
                                "--pattern", name, "--dir", temp], check=True)
            formula = render_formula(tag, Path(temp), TEMPLATE.read_text())
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(formula)
        print(f"Wrote verified formula for {tag}: {args.output}")
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"Homebrew updater failed: {error}\n")


if __name__ == "__main__":
    main()
