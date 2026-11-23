"""Sign and verify the PairShell release manifest with a separate OpenSSH key."""

import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import tempfile


MANIFEST = 'SHA256SUMS.txt'
SIGNATURE = MANIFEST + '.sig'
NAMESPACE = 'pairshell-release-v1'
SIGNER = 'pairshell-release'
ENTRY = re.compile(r'([a-f0-9]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*)')
REPO = Path(__file__).resolve().parents[1]


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def regular_file(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError(f'expected a regular file: {path.name}')
    return path


def read_limited(path, limit):
    with regular_file(path).open('rb') as f:
        value = f.read(limit + 1)
    if len(value) > limit:
        raise ValueError(f'{path.name} exceeds {limit} bytes')
    return value


def manifest_entries(raw):
    entries = {}
    seen = {MANIFEST.casefold(), SIGNATURE.casefold()}
    for line in raw.decode('ascii').splitlines():
        match = ENTRY.fullmatch(line)
        if match is None:
            raise ValueError('invalid SHA-256 manifest entry')
        digest, name = match.groups()
        if name.casefold() in seen or name.endswith('.'):
            raise ValueError(f'invalid or duplicate release file: {name}')
        seen.add(name.casefold())
        entries[name] = digest
    if not entries:
        raise ValueError('empty SHA-256 manifest')
    return entries


def verify_artifacts(release, manifest, artifact=None):
    entries = manifest_entries(manifest)
    if artifact is not None:
        if artifact not in entries:
            raise ValueError('requested artifact is missing from the signed manifest')
        entries = {artifact: entries[artifact]}
    else:
        actual = {p.name for p in release.iterdir() if not p.is_dir()
                  and p.name not in {MANIFEST, SIGNATURE}}
        if actual != entries.keys():
            raise ValueError('release files differ from the manifest')
    for name, expected in entries.items():
        if sha256_file(regular_file(release / name)) != expected:
            raise ValueError(f'SHA-256 mismatch: {name}')


def trusted_key(path, release):
    if path.resolve().is_relative_to(release):
        raise ValueError('trusted public key must come from outside the release directory')
    raw = read_limited(path, 4096).decode('utf-8').strip()
    if '\n' in raw or '\r' in raw:
        raise ValueError('provide one trusted Ed25519 public key')
    fields = raw.split(maxsplit=2)
    if len(fields) < 2 or fields[0] != 'ssh-ed25519' or not re.fullmatch(r'[A-Za-z0-9+/=]+', fields[1]):
        raise ValueError('provide one trusted Ed25519 public key')
    return ' '.join(fields[:2])


def ssh_keygen(arguments, data):
    result = subprocess.run(['ssh-keygen', *arguments], input=data, stdout=subprocess.PIPE, check=False)
    if result.returncode != 0:
        raise ValueError('OpenSSH signature operation failed')
    return result.stdout


def verify_signature(public_key, manifest, signature):
    with tempfile.TemporaryDirectory(prefix='pairshell-signature-') as temporary:
        folder = Path(temporary)
        allowed = folder / 'allowed_signers'
        allowed.write_text(f'{SIGNER} namespaces="{NAMESPACE}" {public_key}\n', encoding='ascii')
        detached = folder / SIGNATURE
        detached.write_bytes(signature)
        ssh_keygen(['-Y', 'verify', '-I', SIGNER, '-n', NAMESPACE, '-f', str(allowed),
                    '-s', str(detached)], manifest)


def sign_release(release, private_key, public_key):
    release = release.resolve(strict=True)
    private_key = regular_file(private_key).resolve(strict=True)
    if private_key.is_relative_to(REPO) or private_key.is_relative_to(release):
        raise ValueError('keep the private release key outside the repository and release directory')
    path = release / SIGNATURE
    if path.exists() or path.is_symlink():
        raise ValueError('release signature already exists')
    key = trusted_key(public_key, release)
    manifest = read_limited(release / MANIFEST, 1 << 20)
    verify_artifacts(release, manifest)
    signature = ssh_keygen(['-Y', 'sign', '-f', str(private_key), '-n', NAMESPACE], manifest)
    verify_signature(key, manifest, signature)
    with path.open('xb') as out:
        out.write(signature)


def verify_release(release, public_key, artifact=None):
    release = release.resolve(strict=True)
    key = trusted_key(public_key, release)
    manifest = read_limited(release / MANIFEST, 1 << 20)
    signature = read_limited(release / SIGNATURE, 16384)
    verify_signature(key, manifest, signature)
    verify_artifacts(release, manifest, artifact)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='action', required=True)
    sign = commands.add_parser('sign')
    verify = commands.add_parser('verify')
    for command in (sign, verify):
        command.add_argument('release', type=Path)
        command.add_argument('--public-key', type=Path, required=True)
    sign.add_argument('--private-key', type=Path, required=True)
    verify.add_argument('--artifact', help='verify only this downloaded filename')
    args = parser.parse_args()
    try:
        if args.action == 'sign':
            sign_release(args.release, args.private_key, args.public_key)
            print('Signed', args.release / SIGNATURE)
        else:
            verify_release(args.release, args.public_key, args.artifact)
            print('Verified release signature and SHA-256:', args.artifact or args.release)
    except (OSError, ValueError) as error:
        parser.exit(1, f'{error}\n')


if __name__ == '__main__':
    main()
