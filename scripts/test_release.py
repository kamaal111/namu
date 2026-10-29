import hashlib
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import release
import update_homebrew


class ReleaseTests(unittest.TestCase):
    def test_reproducible_archive_has_only_executable_with_normalized_metadata(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            binary = root / "namu"
            binary.write_bytes(b"binary contents")
            first, second = root / "first.tar.gz", root / "second.tar.gz"
            release.package_archive(binary, first)
            release.package_archive(binary, second)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            with tarfile.open(first) as archive:
                self.assertEqual(archive.getnames(), ["namu"])
                info = archive.getmember("namu")
                self.assertEqual((info.mode, info.uid, info.gid, info.mtime), (0o755, 0, 0, 0))
                self.assertEqual(archive.extractfile(info).read(), b"binary contents")

    def test_stable_tag_required(self):
        self.assertEqual(release.validate_tag("v0.1.0"), "0.1.0")
        with self.assertRaises(ValueError):
            release.validate_tag("v0.1.0-rc1")
        with self.assertRaises(ValueError):
            release.validate_tag("v01.0.0")
        with self.assertRaises(ValueError):
            release.validate_tag("--help")

    def assets(self, root):
        for target in update_homebrew.TARGETS:
            (root / f"namu_0.1.0_{target}.tar.gz").write_bytes(target.encode())
        (root / "checksums.txt").write_text("".join(
            f"{hashlib.sha256(target.encode()).hexdigest()}  namu_0.1.0_{target}.tar.gz\n"
            for target in update_homebrew.TARGETS))

    def test_publish_uploads_only_missing_assets_after_verifying_existing(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            assets, staging = root / "assets", root / "staging"
            assets.mkdir()
            staging.mkdir()
            self.assets(assets)
            existing = "namu_0.1.0_darwin_arm64.tar.gz"
            (staging / existing).write_bytes((assets / existing).read_bytes())
            with patch.object(release.subprocess, "check_output", return_value=existing + "\n"), \
                    patch.object(release.subprocess, "run") as run:
                release.publish("v0.1.0", assets, staging)
            uploads = [call.args[0][-1] for call in run.call_args_list
                       if call.args[0][1:3] == ["release", "upload"]]
            self.assertEqual(len(uploads), 4)
            self.assertNotIn(str(assets / existing), uploads)

    def test_publish_conflicting_asset_never_uploads(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            assets, staging = root / "assets", root / "staging"
            assets.mkdir()
            staging.mkdir()
            self.assets(assets)
            existing = "checksums.txt"
            (staging / existing).write_text("different")
            with patch.object(release.subprocess, "check_output", return_value=existing + "\n"), \
                    patch.object(release.subprocess, "run") as run:
                with self.assertRaisesRegex(ValueError, "refusing overwrite"):
                    release.publish("v0.1.0", assets, staging)
            uploads = [call for call in run.call_args_list
                       if call.args[0][1:3] == ["release", "upload"]]
            self.assertEqual(uploads, [])

    def test_homebrew_formula_verifies_all_targets(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.assets(root)
            formula = update_homebrew.render_formula("v0.1.0", root, update_homebrew.TEMPLATE.read_text())
            self.assertIn('class Namu < Formula', formula)
            self.assertIn('version "0.1.0"', formula)
            self.assertEqual(formula.count('sha256 "'), 4)
            self.assertNotIn("@SHA_", formula)
            self.assertIn("namu_0.1.0_linux_amd64.tar.gz", formula)

    def test_homebrew_rejects_checksum_mismatch(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.assets(root)
            (root / "namu_0.1.0_linux_arm64.tar.gz").write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "Checksum mismatch"):
                update_homebrew.render_formula("v0.1.0", root, update_homebrew.TEMPLATE.read_text())

    def test_homebrew_rejects_duplicate_checksums(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.assets(root)
            checksums = root / "checksums.txt"
            checksums.write_text(checksums.read_text() + checksums.read_text().splitlines()[0] + "\n")
            with self.assertRaisesRegex(ValueError, "duplicate"):
                update_homebrew.render_formula("v0.1.0", root, update_homebrew.TEMPLATE.read_text())


if __name__ == "__main__":
    unittest.main()
