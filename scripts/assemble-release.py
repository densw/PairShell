"""Collect and verify existing build artifacts; never installs an application."""
from pathlib import Path
import argparse
import io
import json
import plistlib
import re
import shutil
import struct
import tarfile
import zipfile
from release_signing import MANIFEST, sha256_file as sha, sign_release

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--version', default='1.1.3')
    p.add_argument('--output', type=Path, help='Flat release directory (default: dist)')
    p.add_argument('--signing-key', type=Path)
    p.add_argument('--signing-public-key', type=Path)
    args = p.parse_args()
    if (args.signing_key is None) != (args.signing_public_key is None):
        p.error('provide both --signing-key and --signing-public-key')
    assert re.fullmatch(r'\d+\.\d+\.\d+',args.version)
    root=Path(__file__).resolve().parents[1];binary=root/'bin';v=args.version
    release=(args.output if args.output is not None else root/'dist').resolve()
    assert not release.exists(), 'Release output already exists'
    windows=binary/f'PairShell-{v}-Windows-x64'
    assert {f.name for f in windows.iterdir()}=={'PairShell.Customer.exe','PairShell.Merchant.exe','Uninstall.exe','使用说明.txt','runtime'}
    assert sha(windows/'runtime/PairShellBackend.exe')==sha(binary/'pairshell-windows-amd64.exe')
    for role in ['Customer','Merchant']:
        installer=binary/f'PairShell-{v}-Windows-x64-{role}-Setup.exe'
        assert installer.read_bytes()[:2]==b'MZ'
    for suffix in ['Windows-x64.zip','macOS-universal.zip']:
        with zipfile.ZipFile(binary/f'PairShell-{v}-{suffix}') as z:
            assert z.testzip() is None
            assert not any(n.startswith('/') or '..' in Path(n).parts for n in z.namelist())
            if suffix.startswith('macOS'):
                props=plistlib.loads(z.read('PairShell.app/Contents/Info.plist'))
                assert props['CFBundleShortVersionString']==v
                assert props['CFBundleIdentifier']=='org.pairshell.desktop'
                for name in ['Contents/MacOS/PairShellCustomer','Contents/Helpers/PairShellBackend']:
                    path='PairShell.app/'+name;data=z.read(path)
                    assert data[:4]==bytes.fromhex('cafebabe')
                    count=struct.unpack_from('>I',data,4)[0]
                    assert count==2
                    assert {struct.unpack_from('>I',data,8+20*i)[0] for i in range(count)}=={0x01000007,0x0100000c}
                    assert z.getinfo(path).external_attr>>16&0o111==0o111
    dmg=binary/f'PairShell-{v}-macOS-universal.dmg'
    with dmg.open('rb') as f:
        f.seek(-512,2)
        assert f.read(4)==b'koly', 'Mac distribution must be a real UDIF DMG'
    release.mkdir()
    for suffix in ['Windows-x64.zip','Windows-x64-Customer-Setup.exe','Windows-x64-Merchant-Setup.exe','macOS-universal.dmg']:
        shutil.copyfile(binary/f'PairShell-{v}-{suffix}',release/f'PairShell-{v}-{suffix}')
    for arch in ['amd64','arm64']:
        output=release/f'PairShell-{v}-Linux-{arch}.tar.gz'
        files={'pairshell':(binary/f'pairshell-linux-{arch}').read_bytes(),'pairshelld':(binary/f'pairshelld-linux-{arch}').read_bytes(),'THIRD_PARTY_NOTICES.md':(root/'docs/THIRD_PARTY_NOTICES.md').read_bytes()}
        with tarfile.open(output,'w:gz') as tar:
            for name,data in files.items():
                info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o755 if name in ['pairshell','pairshelld'] else 0o644;tar.addfile(info,io.BytesIO(data))
        with tarfile.open(output) as tar:
            assert tar.extractfile('pairshell').read()==files['pairshell']
            assert tar.extractfile('pairshelld').read()==files['pairshelld']
    report={'version':v,'checks':{'zip_crc':True,'windows_shared_backend_matches_build':True,'windows_root_files':4,'mac_app_count':1,'mac_distribution':'UDIF DMG','mac_architectures':['x86_64','arm64'],'mac_executable_bits':True,'linux_archive_contents':True},'not_verified':['Windows install/uninstall execution','macOS native startup, DMG mounting, Gatekeeper and migration to a second Mac','Apple Developer ID notarization','Live remote session end-to-end acceptance for this release']}
    (release/'verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (release/MANIFEST).write_text(''.join(sha(f)+'  '+f.name+'\n' for f in sorted(release.iterdir()) if f.is_file()),encoding='utf-8')
    if args.signing_key is not None:
        try:
            sign_release(release, args.signing_key, args.signing_public_key)
        except (OSError, ValueError) as error:
            p.exit(1, f'Release signing failed: {error}\n')
    print('RELEASE_READY',release)

if __name__=='__main__':main()
