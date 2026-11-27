"""Check DMG independently with 7-Zip and compare the extracted signed bundle."""
from pathlib import Path
import argparse
import hashlib
import plistlib
import subprocess
import tempfile
import zipfile
import shutil
from verify_macho_signature import verify_binary
from build_tools import find_tool

p=argparse.ArgumentParser();p.add_argument('--version',default='1.1.3');args=p.parse_args()
root=Path(__file__).resolve().parents[1]
seven=find_tool('7z', '7zip/7z.exe')
dmg=root/f'bin/PairShell-{args.version}-macOS-universal.dmg'
with dmg.open('rb') as f:
    f.seek(-512,2);assert f.read(4)==b'koly'
subprocess.run([str(seven),'t',str(dmg)],check=True,stdout=subprocess.DEVNULL)
listing=subprocess.check_output([str(seven),'l','-slt','-sccUTF-8',str(dmg)]).decode('utf-8').replace('\r\n','\n')
assert 'Method = HFS+' in listing
assert any('Path = ' in block and block.split('Path = ',1)[1].splitlines()[0].replace('\\','/').split('/')[-1]=='Applications'
           and 'Mode = lrwxr-xr-x' in block for block in listing.split('\n\n'))
assert listing.count('Mode = -rwxr-xr-x')==2, 'Both Mac binaries must remain executable'
app_paths=[line.removeprefix('Path = ').replace('\\','/') for line in listing.splitlines()
           if line.startswith('Path = ') and line.removeprefix('Path = ').replace('\\','/').split('/')[-1]=='PairShell.app']
assert len(app_paths)==1, 'DMG must contain exactly one native App'
stage=Path(tempfile.mkdtemp(prefix='pairshell-dmg-test-'))
subprocess.run([str(seven),'x',str(dmg),'-o'+str(stage),app_paths[0]+'/*','-r','-y'],check=True,stdout=subprocess.DEVNULL)
apps=list(stage.rglob('PairShell.app'))
assert len(apps)==1, 'DMG must contain exactly one native App'
app=apps[0]
with zipfile.ZipFile(root/f'bin/PairShell-{args.version}-macOS-universal.zip') as archive:
    for name in archive.namelist():
        if name.startswith('PairShell.app/') and not name.endswith('/'):
            assert hashlib.sha256((app.parent/name).read_bytes()).digest()==hashlib.sha256(archive.read(name)).digest(),name
assert plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleShortVersionString']==args.version
verify_binary(app/'Contents/MacOS/PairShellCustomer',app)
verify_binary(app/'Contents/Helpers/PairShellBackend')
shutil.rmtree(stage)
print('DMG: PASS (UDIF CRC, HFS+, Applications link, executable modes, bundle hashes and signatures)')
