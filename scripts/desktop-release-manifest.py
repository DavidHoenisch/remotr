#!/usr/bin/env python3
"""Generate and validate the Linux-only Remotr Desktop artifact manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys
from typing import Any


VERSION_PATTERN = re.compile(r"^[0-9][0-9A-Za-z.+~_-]*$")


class ManifestError(ValueError):
    pass


def load_json(path: Path, label: str) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ManifestError(f"cannot read {label} {path}: {error}") from error
    if not isinstance(value, dict):
        raise ManifestError(f"{label} must be a JSON object")
    return value


def target_inventory(path: Path) -> tuple[dict[str, Any], dict[tuple[str, str, str], dict[str, Any]]]:
    inventory = load_json(path, "package target inventory")
    if inventory.get("schemaVersion") != 1 or inventory.get("product") != "Remotr Desktop":
        raise ManifestError("package target inventory has an unsupported identity or schema")
    raw_targets = inventory.get("artifacts")
    if not isinstance(raw_targets, list) or not raw_targets:
        raise ManifestError("package target inventory has no advertised artifacts")
    signed_output = inventory.get("signedReleaseOutput")
    if not isinstance(signed_output, dict):
        raise ManifestError("package target inventory has no signing classification")
    if signed_output.get("configured") is not False or signed_output.get("policy") != "not-configured":
        raise ManifestError("package target inventory does not explicitly classify signed release output")

    targets: dict[tuple[str, str, str], dict[str, Any]] = {}
    for index, target in enumerate(raw_targets):
        if not isinstance(target, dict):
            raise ManifestError(f"package target {index} is not an object")
        key = (target.get("os"), target.get("architecture"), target.get("packageFormat"))
        if key[0] != "linux":
            raise ManifestError(f"package target {index} violates the Linux-only desktop release policy")
        if not all(isinstance(part, str) and part for part in key):
            raise ManifestError(f"package target {index} has an incomplete OS/architecture/format")
        if key in targets:
            raise ManifestError(f"duplicate package target {'/'.join(key)}")
        required = target.get("requiredEvidence")
        if not isinstance(required, list) or required != ["build", "install", "launch", "remove"]:
            raise ManifestError(f"package target {'/'.join(key)} does not require complete native evidence")
        if not isinstance(target.get("evidenceCommand"), str) or not target["evidenceCommand"]:
            raise ManifestError(f"package target {'/'.join(key)} has no evidence command")
        publication = target.get("publication")
        signing_status = target.get("signingStatus")
        release_eligible = target.get("releaseEligible")
        if publication == "ci-development-artifact":
            if signing_status != "unsigned" or release_eligible is not False:
                raise ManifestError(f"package target {'/'.join(key)} is not classified as an unsigned non-release artifact")
        elif publication == "github-release-asset":
            if signing_status != "unsigned" or release_eligible is not True:
                raise ManifestError(f"package target {'/'.join(key)} is not classified as an unsigned release asset")
        else:
            raise ManifestError(f"package target {'/'.join(key)} has an unsupported publication class")
        targets[key] = target
    return inventory, targets


def digest(path: Path) -> str:
    checksum = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            checksum.update(chunk)
    return checksum.hexdigest()


def artifact_filename(version: str, architecture: str, package_format: str) -> str:
    extension = {
        "deb": "deb",
        "flatpak": "flatpak",
    }.get(package_format)
    if extension is None:
        raise ManifestError(f"cannot derive filename for unsupported package format {package_format}")
    return f"remotr-desktop_{version}_{architecture}.{extension}"


def generate(args: argparse.Namespace) -> None:
    inventory, targets = target_inventory(args.targets)
    if not VERSION_PATTERN.fullmatch(args.version):
        raise ManifestError("version is not a bounded package-safe value")
    key = (args.os, args.architecture, args.package_format)
    target = targets.get(key)
    if target is None:
        raise ManifestError(f"artifact target {'/'.join(key)} is not advertised with native evidence")

    artifact = args.artifact.resolve()
    if not artifact.is_file() or artifact.is_symlink():
        raise ManifestError(f"artifact is not a regular file: {artifact}")
    expected_name = artifact_filename(args.version, args.architecture, args.package_format)
    if artifact.name != expected_name:
        raise ManifestError(f"artifact filename is {artifact.name!r}; expected {expected_name!r}")

    evidence: dict[str, str] = {"command": target["evidenceCommand"]}
    for name in target["requiredEvidence"]:
        evidence[name] = "passed"

    manifest = {
        "schemaVersion": 1,
        "product": "Remotr Desktop",
        "version": args.version,
        "signedReleaseOutput": inventory["signedReleaseOutput"],
        "artifacts": [
            {
                "os": args.os,
                "architecture": args.architecture,
                "packageFormat": args.package_format,
                "publication": target["publication"],
                "signingStatus": target["signingStatus"],
                "releaseEligible": target["releaseEligible"],
                "file": artifact.name,
                "sha256": digest(artifact),
                "size": artifact.stat().st_size,
                "evidence": evidence,
            }
        ],
    }

    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_name(output.name + ".tmp")
    temporary.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.chmod(temporary, 0o644)
    os.replace(temporary, output)
    print(f"desktop release manifest: wrote {output}")


def require_string(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value:
        raise ManifestError(f"{label} must be a non-empty string")
    return value


def check(args: argparse.Namespace) -> None:
    inventory, targets = target_inventory(args.targets)
    manifest = load_json(args.manifest, "release manifest")
    if manifest.get("schemaVersion") != 1 or manifest.get("product") != "Remotr Desktop":
        raise ManifestError("release manifest has an unsupported identity or schema")
    version = require_string(manifest.get("version"), "release manifest version")
    if not VERSION_PATTERN.fullmatch(version):
        raise ManifestError("release manifest version is not a bounded package-safe value")

    signed_output = manifest.get("signedReleaseOutput")
    expected_signed = inventory.get("signedReleaseOutput")
    if signed_output != expected_signed:
        raise ManifestError("release manifest signing classification does not match the target inventory")

    raw_artifacts = manifest.get("artifacts")
    if not isinstance(raw_artifacts, list) or not raw_artifacts:
        raise ManifestError("release manifest has no artifacts")

    artifact_dir = args.artifact_dir.resolve()
    if not artifact_dir.is_dir():
        raise ManifestError(f"artifact directory does not exist: {artifact_dir}")
    manifest_path = args.manifest.resolve()
    if manifest_path != artifact_dir / "release-manifest.json" or not manifest_path.is_file() or manifest_path.is_symlink():
        raise ManifestError("checked release manifest must be the regular release-manifest.json in the artifact directory")
    declared_files: set[str] = set()
    declared_targets: set[tuple[str, str, str]] = set()

    for index, artifact in enumerate(raw_artifacts):
        if not isinstance(artifact, dict):
            raise ManifestError(f"release artifact {index} is not an object")
        operating_system = require_string(artifact.get("os"), f"release artifact {index} OS")
        if operating_system != "linux":
            raise ManifestError(f"release artifact {index} violates the Linux-only desktop release policy")
        architecture = require_string(artifact.get("architecture"), f"release artifact {index} architecture")
        package_format = require_string(artifact.get("packageFormat"), f"release artifact {index} package format")
        key = (operating_system, architecture, package_format)
        target = targets.get(key)
        if target is None:
            raise ManifestError(f"artifact target {'/'.join(key)} is not advertised with native evidence")
        if key in declared_targets:
            raise ManifestError(f"duplicate release artifact target {'/'.join(key)}")
        declared_targets.add(key)

        for field in ("publication", "signingStatus", "releaseEligible"):
            if artifact.get(field) != target.get(field):
                raise ManifestError(f"release artifact {index} {field} does not match its advertised target")

        evidence = artifact.get("evidence")
        if not isinstance(evidence, dict) or evidence.get("command") != target.get("evidenceCommand"):
            raise ManifestError(f"release artifact {index} evidence command does not match its advertised target")
        for name in target["requiredEvidence"]:
            if evidence.get(name) != "passed":
                raise ManifestError(f"release artifact {index} {name} evidence is not passed")

        filename = require_string(artifact.get("file"), f"release artifact {index} filename")
        if Path(filename).name != filename or "/" in filename or "\\" in filename:
            raise ManifestError(f"release artifact {index} filename escapes the artifact directory")
        if filename != artifact_filename(version, architecture, package_format):
            raise ManifestError(f"release artifact {index} filename does not match version and target")
        if filename in declared_files:
            raise ManifestError(f"duplicate release artifact file {filename}")
        declared_files.add(filename)

        artifact_path = artifact_dir / filename
        if not artifact_path.is_file() or artifact_path.is_symlink():
            raise ManifestError(f"declared artifact is not a regular file: {filename}")
        actual_size = artifact_path.stat().st_size
        if artifact.get("size") != actual_size:
            raise ManifestError(f"artifact {filename} size mismatch")
        actual_digest = digest(artifact_path)
        if artifact.get("sha256") != actual_digest:
            raise ManifestError(f"artifact {filename} SHA-256 mismatch")

    allowed_files = declared_files | {"release-manifest.json"}
    for child in artifact_dir.iterdir():
        if child.name not in allowed_files:
            raise ManifestError(f"undeclared artifact in upload directory: {child.name}")
        if child.is_dir() or child.is_symlink():
            raise ManifestError(f"artifact upload entry is not a regular file: {child.name}")

    print(f"desktop release manifest: checked {len(raw_artifacts)} Linux artifact(s)")


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description=__doc__)
    commands = root.add_subparsers(dest="command", required=True)

    generate_parser = commands.add_parser("generate")
    generate_parser.add_argument("--targets", type=Path, required=True)
    generate_parser.add_argument("--artifact", type=Path, required=True)
    generate_parser.add_argument("--version", required=True)
    generate_parser.add_argument("--os", required=True)
    generate_parser.add_argument("--architecture", required=True)
    generate_parser.add_argument("--format", dest="package_format", required=True)
    generate_parser.add_argument("--output", type=Path, required=True)
    generate_parser.set_defaults(handler=generate)

    check_parser = commands.add_parser("check")
    check_parser.add_argument("--targets", type=Path, required=True)
    check_parser.add_argument("--manifest", type=Path, required=True)
    check_parser.add_argument("--artifact-dir", type=Path, required=True)
    check_parser.set_defaults(handler=check)
    return root


def main() -> int:
    args = parser().parse_args()
    try:
        args.handler(args)
    except ManifestError as error:
        print(f"desktop release manifest: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
