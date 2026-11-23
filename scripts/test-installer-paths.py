"""Execute the actual NSIS path functions against absent paths, without installing."""
from pathlib import Path
import re
import subprocess
import tempfile

root=Path(__file__).resolve().parents[1]
stage=Path(tempfile.mkdtemp(prefix='installer-path-tests-',dir=root/'.tools'))
source=(root/'native/windows/installer.nsi').read_text(encoding='utf-8')
functions='\n'.join(re.search(r'^Function '+name+r'\r?\n.*?^FunctionEnd',source,re.M|re.S).group() for name in ['NormalizeDirectory','CheckDirectory'])
target=stage/'尚未创建'/'nested'/'PairShell'
cases=[('D:\\Tools\\','D:\\Tools\\PairShell'),('D:\\Tools\\PairShell','D:\\Tools\\PairShell'),('D:\\Tools\\PairShell\\','D:\\Tools\\PairShell'),('D:\\','D:\\PairShell'),(str(target),str(target))]
script='Unicode true\n!include "FileFunc.nsh"\nRequestExecutionLevel user\nSilentInstall silent\nVar InstallBase\nVar PathError\nOutFile "'+str(stage/'probe.exe')+'"\n'+functions+'\nSection\nFileOpen $9 "'+str(stage/'result.txt')+'" w\n'
for given,_ in cases:
    script+='StrCpy $InstallBase "'+given+'"\n'
    for _ in range(2):
        script+='Call NormalizeDirectory\nFileWriteUTF16LE $9 "$InstallBase$\\t$INSTDIR$\\t$PathError$\\r$\\n"\n'
# Exercise the same final validator and directory-creation instruction as installation.
script+='StrCpy $InstallBase "'+str(target)+'"\nCall CheckDirectory\nStrCmp $PathError "" valid\nSetErrorLevel 2\nAbort\nvalid:\nSetOutPath "$INSTDIR"\nFileOpen $8 "$INSTDIR\\probe.txt" w\nFileWrite $8 "written"\nFileClose $8\nFileClose $9\nSectionEnd\n'
(stage/'probe.nsi').write_text(script,encoding='utf-8-sig')
subprocess.run([str(root/'.tools/nsis/nsis-3.13/makensis.exe'),'/V2',str(stage/'probe.nsi')],check=True)
result=subprocess.run([str(stage/'probe.exe')],timeout=30)
rows=(stage/'result.txt').read_text(encoding='utf-16-le').splitlines()
for i,(_,expected) in enumerate(cases):
    for row in rows[2*i:2*i+2]:
        assert row.split('\t')==[expected,expected,''],f'Unexpected normalization: {row!r}'
assert len(rows)==len(cases)*2
assert result.returncode==0 and (target/'probe.txt').read_text()=='written'
print('Installer paths: PASS (browse parent, existing suffix, root, absent Unicode hierarchy, repeat validation, actual fixture write)')
