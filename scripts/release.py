#!/usr/bin/env python3
"""Deterministic release archives and immutable GitHub asset publication."""

import argparse
import gzip
import hashlib
import io
from pathlib import Path
import re
import subprocess
import tarfile


TARGETS = ("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64")

def validate_tag(tag: str) -> str:
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Expected a stable vX.Y.Z tag")
    return tag[1:]


def archive_names(tag: str) -> list[str]:
    version = validate_tag(tag)
    return [f"namu_{version}_{target}.tar.gz" for target in TARGETS]


def package_archive(binary: Path, destination: Path) -> None:
    content = binary.read_bytes()
    with destination.open("wb") as output:
        with gzip.GzipFile(fileobj=output, mode="wb", filename="", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                info = tarfile.TarInfo("namu")
                info.size = len(content)
                info.mode = 0o755
                archive.addfile(info, io.BytesIO(content))


def publish(tag: str, directory: Path, staging: Path) -> None:
    validate_tag(tag)
    subprocess.run(
        ["gh", "release", "view", tag, "--json", "isDraft,isPrerelease", "--jq",
         'if .isDraft or .isPrerelease then error("Expected published stable release") else . end'],
        check=True, stdout=subprocess.DEVNULL,
    )
    existing = set(subprocess.check_output(
        ["gh", "release", "view", tag, "--json", "assets", "--jq", ".assets[].name"], text=True,
    ).splitlines())
    expected = archive_names(tag) + ["checksums.txt"]
    for name in expected:
        if not (directory / name).is_file():
            raise ValueError(f"Missing built asset: {name}")
        if name in existing:
            subprocess.run(["gh", "release", "download", tag, "--pattern", name,
                            "--dir", str(staging)], check=True)
            if (directory / name).read_bytes() != (staging / name).read_bytes():
                raise ValueError(f"Existing {name} differs; refusing overwrite")
    for name in expected:
        if name not in existing:
            subprocess.run(["gh", "release", "upload", tag, str(directory / name)], check=True)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("targets")
    checksum = commands.add_parser("checksums")
    checksum.add_argument("tag")
    checksum.add_argument("directory", type=Path)
    validate = commands.add_parser("validate-tag")
    validate.add_argument("tag")
    package = commands.add_parser("package")
    package.add_argument("binary", type=Path)
    package.add_argument("archive", type=Path)
    upload = commands.add_parser("publish")
    upload.add_argument("tag")
    upload.add_argument("directory", type=Path)
    upload.add_argument("staging", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "targets":
            print(" ".join(TARGETS))
        elif args.command == "checksums":
            lines = []
            for name in sorted(archive_names(args.tag)):
                digest = hashlib.sha256((args.directory / name).read_bytes()).hexdigest()
                lines.append(f"{digest}  {name}\n")
            (args.directory / "checksums.txt").write_text("".join(lines))
        elif args.command == "validate-tag":
            validate_tag(args.tag)
        elif args.command == "package":
            package_archive(args.binary, args.archive)
        else:
            publish(args.tag, args.directory, args.staging)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"Release helper failed: {error}\n")


if __name__ == "__main__":
    main()
