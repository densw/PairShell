"""Read-only/extraction integration checks; does not run either installer."""
from pathlib import Path
import argparse
import hashlib
import subprocess
import tempfile
import zipfile
import shutil
from build_tools import find_tool

parser=argparse.ArgumentParser();parser.add_argument('--version',default='1.1.3');args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
seven=find_tool('7z', '7zip/7z.exe')
backend=root/f'bin/PairShell-{args.version}-Windows-x64/runtime/PairShellBackend.exe'
digest=lambda p:hashlib.sha256(p.read_bytes()).digest()
with zipfile.ZipFile(root/f'bin/PairShell-{args.version}-Windows-x64.zip') as archive:
    assert archive.read('Uninstall.exe')[:2]==b'MZ', 'Portable package needs a real uninstall EXE'
    assert archive.read('runtime/.pairshell-portable')==b'PairShell portable', 'Portable uninstall must have its own directory marker'
print('Portable package: uninstall EXE and ownership marker PASS')
for role in ['Customer','Merchant']:
    installer=root/f'bin/PairShell-{args.version}-Windows-x64-{role}-Setup.exe'
    assert installer.is_file(), f'Missing separate {role} installer'
    stage=Path(tempfile.mkdtemp(prefix='pairshell-installer-test-'))
    subprocess.run([str(seven),'x',str(installer),'-o'+str(stage),'-y'],check=True,stdout=subprocess.DEVNULL)
    assert (stage/'PairShell.exe').read_bytes()[:2]==b'MZ', 'Both installers must expose the same PairShell.exe name'
    assert not list(stage.glob('PairShell.*.exe')), 'Installed EXE names must not contain role suffixes'
    subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-File',str(root/'scripts/test-launcher-default.ps1'),'-Executable',str(stage/'PairShell.exe'),'-ExpectedRole',role.lower()],check=True)
    assert (stage/'Uninstall.exe').read_bytes()[:2]==b'MZ', 'Missing uninstall EXE'
    assert not (stage/'runtime/.pairshell-portable').exists(), 'Installed-role payload must not masquerade as portable'
    assert digest(stage/'runtime/PairShellBackend.exe')==digest(backend), 'Wrong shared backend'
    assert digest(stage/'runtime/PairShell.exe')==digest(root/f'bin/PairShell-{args.version}-Windows-x64/runtime/PairShell.exe'), 'Wrong native core'
    shutil.rmtree(stage)
    print(role+': installer payload PASS')
source=(root/'native/windows/installer.nsi').read_text(encoding='utf-8')
assert 'Function .onVerifyInstDir' not in source, 'Must not reject parent folders during browse'
assert 'nsDialogs::SelectFolderDialog' in source
assert '${NSD_SetText} $DirectoryInput "$InstallBase\\"' in source, 'Browse result must immediately update the path field'
assert 'StrCpy $InstallBase "$InstallBase\\PairShell"' in source
assert 'StrCpy $INSTDIR $InstallBase' in source
assert '$InstallBase\\@ROLE@' not in source, 'No role subdirectory is permitted'
print('Directory selection: shared PairShell target and immediate browse-field update PASS')
