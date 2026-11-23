"""Read-only/extraction integration checks; does not run either installer."""
from pathlib import Path
import hashlib
import subprocess
import tempfile

root=Path(__file__).resolve().parents[1]
seven=root/'.tools/release-0.10.1/mac/7zip-full/7z.exe'
backend=root/'bin/PairShell-1.1.0-Windows-x64/runtime/PairShellBackend.exe'
digest=lambda p:hashlib.sha256(p.read_bytes()).digest()
for role in ['Customer','Merchant']:
    installer=root/f'bin/PairShell-1.1.0-Windows-x64-{role}-Setup.exe'
    assert installer.is_file(), f'Missing separate {role} installer'
    stage=Path(tempfile.mkdtemp(prefix='installer-test-',dir=root/'.tools'))
    subprocess.run([str(seven),'x',str(installer),'-o'+str(stage),'-y'],check=True,stdout=subprocess.DEVNULL)
    assert {p.name for p in stage.glob('PairShell.*.exe')}=={f'PairShell.{role}.exe'}, 'Wrong role launchers'
    assert (stage/'Uninstall.exe').read_bytes()[:2]==b'MZ', 'Missing uninstall EXE'
    assert digest(stage/'runtime/PairShellBackend.exe')==digest(backend), 'Wrong shared backend'
    assert digest(stage/'runtime/PairShell.exe')==digest(root/'bin/PairShell-1.1.0-Windows-x64/runtime/PairShell.exe'), 'Wrong native core'
    print(role+': installer payload PASS')
source=(root/'native/windows/installer.nsi').read_text(encoding='utf-8')
assert 'Function .onVerifyInstDir' not in source, 'Must not reject parent folders during browse'
assert 'MUI_DIRECTORYPAGE_CUSTOMFUNCTION_LEAVE ValidateDirectory' in source
assert 'MUI_DIRECTORYPAGE_VERIFYONLEAVE' in source
assert 'StrCpy $InstallBase "$InstallBase\\PairShell"' in source, 'Bare drive root must gain PairShell'
assert 'StrCpy $INSTDIR "$InstallBase\\@ROLE@"' in source
assert 'Software\\PairShell\\@ROLE@' in source, 'Role installs need separate registration'
print('Directory selection: callback stages and drive-root normalization PASS (native UI pending)')
