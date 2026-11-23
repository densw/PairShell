"""Exercise release signing with disposable keys and synthetic artifacts."""

import hashlib
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from release_signing import MANIFEST, SIGNATURE, sign_release, verify_release


@unittest.skipUnless(shutil.which('ssh-keygen'), 'requires OpenSSH')
class TestReleaseSigning(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='pairshell-release-test-')
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.release = self.directory / 'release'
        self.release.mkdir()
        self.artifact = self.release / 'PairShell-Test-Windows-x64.zip'
        self.artifact.write_bytes(b'synthetic release bytes')
        second = self.release / 'PairShell-Test-macOS.zip'
        second.write_bytes(b'second synthetic release payload')
        self.manifest = self.release / MANIFEST
        self.manifest.write_text(''.join(
            f'{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n'
            for path in (self.artifact, second)
        ), encoding='ascii')
        self.key = self.create_key('signer')
        self.public_key = self.key.with_suffix('.pub')

    def create_key(self, name):
        key = self.directory / name
        subprocess.run(['ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(key)], check=True)
        return key

    def test_valid_release_and_single_artifact(self):
        sign_release(self.release, self.key, self.public_key)
        verify_release(self.release, self.public_key)
        (self.release / 'PairShell-Test-macOS.zip').unlink()
        verify_release(self.release, self.public_key, self.artifact.name)
        with self.assertRaisesRegex(ValueError, 'release files differ'):
            verify_release(self.release, self.public_key)

    def test_wrong_public_key_rejected(self):
        sign_release(self.release, self.key, self.public_key)
        other = self.create_key('other')
        with self.assertRaisesRegex(ValueError, 'OpenSSH signature'):
            verify_release(self.release, other.with_suffix('.pub'))

    def test_mismatched_signing_key_does_not_leave_signature(self):
        other = self.create_key('other')
        with self.assertRaisesRegex(ValueError, 'OpenSSH signature'):
            sign_release(self.release, other, self.public_key)
        self.assertFalse((self.release / SIGNATURE).exists())

    def test_modified_manifest_rejected(self):
        sign_release(self.release, self.key, self.public_key)
        self.manifest.write_bytes(self.manifest.read_bytes().replace(b'Windows', b'Wind0ws'))
        with self.assertRaisesRegex(ValueError, 'OpenSSH signature'):
            verify_release(self.release, self.public_key)

    def test_modified_artifact_rejected(self):
        sign_release(self.release, self.key, self.public_key)
        self.artifact.write_bytes(b'changed bytes')
        with self.assertRaisesRegex(ValueError, 'SHA-256 mismatch'):
            verify_release(self.release, self.public_key)


if __name__ == '__main__':
    unittest.main()
