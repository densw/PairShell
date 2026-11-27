"""Run the portable uninstaller only against disposable .tools fixtures."""
from pathlib import Path
import argparse
import shutil
import subprocess
import tempfile
import time

p=argparse.ArgumentParser();p.add_argument('--version',default='1.1.3');args=p.parse_args()
root=Path(__file__).resolve().parents[1]
package=root/f'bin/PairShell-{args.version}-Windows-x64'
tools=(root/'.tools').resolve()
fixture=Path(tempfile.mkdtemp(prefix='portable-uninstall-test-',dir=tools)).resolve()
assert fixture.is_relative_to(tools)

def make_case(name,marker):
    target=fixture/name;target.mkdir()
    owned=[]
    for original in package.rglob('*'):
        if not original.is_file() or original.name=='Uninstall.exe':continue
        relative=original.relative_to(package);dest=target/relative
        dest.parent.mkdir(parents=True,exist_ok=True)
        dest.write_bytes(original.read_bytes() if relative.as_posix()=='runtime/.pairshell-portable' else b'fixture data')
        owned.append(dest)
    (target/'runtime/.pairshell-portable').write_text(marker,encoding='utf-8')
    shutil.copyfile(package/'Uninstall.exe',target/'Uninstall.exe')
    (target/'keep-user-file.txt').write_text('preserve me',encoding='utf-8')
    (target/'runtime/keep-user-file.txt').write_text('preserve nested data',encoding='utf-8')
    return target,owned

invalid,invalid_owned=make_case('invalid','not a PairShell package')
subprocess.run([str(invalid/'Uninstall.exe'),'/S'],cwd=invalid,timeout=30)
time.sleep(1)
assert all(path.exists() for path in invalid_owned), 'Unrecognized directory must remain untouched'
valid,valid_owned=make_case('valid','PairShell portable')
subprocess.run([str(valid/'Uninstall.exe'),'/S'],cwd=valid,timeout=30)
deadline=time.monotonic()+15
while any(path.exists() for path in valid_owned) and time.monotonic()<deadline:
    time.sleep(.2)
assert not any(path.exists() for path in valid_owned), 'Owned files were not fully removed'
assert (valid/'keep-user-file.txt').read_text(encoding='utf-8')=='preserve me', 'Unrelated files must survive'
assert (valid/'runtime/keep-user-file.txt').read_text(encoding='utf-8')=='preserve nested data', 'Unrelated runtime files must survive'
assert (package/'Uninstall.exe').is_file(), 'Actual package must remain untouched'
print('Portable uninstall: PASS (invalid marker rejected, fixture payload removed, unrelated data preserved)')
