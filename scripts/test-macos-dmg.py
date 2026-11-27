"""Check DMG independently with 7-Zip and compare the extracted signed bundle."""
from pathlib import Path
import argparse
import hashlib
import plistlib
import subprocess
import tempfile
import zipfile
from verify_macho_signature import verify_binary

p=argparse.ArgumentParser();p.add_argument('--version',default='1.1.3');args=p.parse_args()
root=Path(__file__).resolve().parents[1]
seven=root/'.tools/release-0.10.1/mac/7zip-full/7z.exe'
dmg=root/f'bin/PairShell-{args.version}-macOS-universal.dmg'
with dmg.open('rb') as f:
    f.seek(-512,2);assert f.read(4)==b'koly'
subprocess.run([str(seven),'t',str(dmg)],check=True,stdout=subprocess.DEVNULL)
listing=subprocess.check_output([str(seven),'l','-slt',str(dmg)]).decode('utf-8',errors='replace')
assert 'Method = HFS+' in listing
assert 'Path = PairShell\\Applications' in listing and 'Mode = lrwxr-xr-x' in listing
assert listing.count('Mode = -rwxr-xr-x')==2, 'Both Mac binaries must remain executable'
stage=Path(tempfile.mkdtemp(prefix='dmg-test-',dir=root/'.tools'))
subprocess.run([str(seven),'x',str(dmg),'-o'+str(stage),'PairShell/PairShell.app/*','-r','-y'],check=True,stdout=subprocess.DEVNULL)
app=stage/'PairShell/PairShell.app'
with zipfile.ZipFile(root/f'bin/PairShell-{args.version}-macOS-universal.zip') as archive:
    for name in archive.namelist():
        if name.startswith('PairShell.app/') and not name.endswith('/'):
            assert hashlib.sha256((stage/'PairShell'/name).read_bytes()).digest()==hashlib.sha256(archive.read(name)).digest(),name
assert plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleShortVersionString']==args.version
verify_binary(app/'Contents/MacOS/PairShellCustomer',app)
verify_binary(app/'Contents/Helpers/PairShellBackend')
print('DMG: PASS (UDIF CRC, HFS+, Applications link, executable modes, bundle hashes and signatures)')
