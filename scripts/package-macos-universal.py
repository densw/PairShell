"""Assemble one portable universal App locally, using the cached native shell.

Requires prebuilt universal Cocoa shell and rcodesign; never connects to a Mac.
"""
from pathlib import Path
import argparse, hashlib, plistlib, re, shutil, struct, subprocess, tempfile, zipfile
from verify_macho_signature import verify_binary

def macho_architectures(data):
    magic=struct.unpack_from('>I',data)[0]
    if magic==0xcafebabe:
        count=struct.unpack_from('>I',data,4)[0]
        return [struct.unpack_from('>IIIII',data,8+20*i)[0] for i in range(count)]
    if data[:4]==bytes.fromhex('cffaedfe'):
        return [struct.unpack_from('<I',data,4)[0]]
    raise ValueError('Unsupported Mach-O')

def fat_binary(paths):
    slices=[]
    for p in paths:
        data=p.read_bytes();assert data[:4]==bytes.fromhex('cffaedfe')
        cpu,sub=struct.unpack_from('<II',data,4);slices.append((cpu,sub,data))
    assert {cpu for cpu,_,_ in slices}=={0x01000007,0x0100000c}
    offset=16384;records=[]
    for cpu,sub,data in slices:
        records.append((cpu,sub,offset,len(data),14));offset=(offset+len(data)+16383)&~16383
    result=bytearray(offset);struct.pack_into('>II',result,0,0xcafebabe,len(slices))
    for i,(record,(_,_,data)) in enumerate(zip(records,slices)):
        struct.pack_into('>IIIII',result,8+i*20,*record);result[record[2]:record[2]+len(data)]=data
    return result

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--version',default='1.1.0');args=parser.parse_args()
    if not re.fullmatch(r'\d+\.\d+\.\d+', args.version):
        parser.error('Version must use major.minor.patch')
    repo=Path(__file__).resolve().parents[1]
    source=repo/'.tools/release-0.10.1/mac/zip-root/PairShell.app'
    signer=repo/'.tools/release-0.10.1/mac/apple-codesign-0.29.0-x86_64-pc-windows-msvc/rcodesign.exe'
    output=repo/'bin'/('PairShell-'+args.version+'-macOS-universal.zip')
    stage=Path(tempfile.mkdtemp(prefix='mac-universal-'+args.version+'-',dir=repo/'.tools'))
    app=stage/'PairShell.app';shutil.copytree(source,app)
    shell=app/'Contents/MacOS/PairShellCustomer'
    assert set(macho_architectures(shell.read_bytes()))=={0x01000007,0x0100000c}
    backend=app/'Contents/Helpers/PairShellBackend'
    backend.write_bytes(fat_binary([repo/'bin/pairshell-darwin-amd64',repo/'bin/pairshell-darwin-arm64']))
    info=app/'Contents/Info.plist';props=plistlib.loads(info.read_bytes())
    props.update(CFBundleName='PairShell',CFBundleDisplayName='PairShell',CFBundleIdentifier='org.pairshell.desktop',CFBundleShortVersionString=args.version,CFBundleVersion=args.version.replace('.',''))
    info.write_bytes(plistlib.dumps(props))
    shutil.copyfile(repo/'docs/THIRD_PARTY_NOTICES.md',app/'Contents/Resources/THIRD_PARTY_NOTICES.md')
    subprocess.run([str(signer),'sign',str(app)],check=True)
    # rcodesign verify rejects the empty CMS blob used by ad-hoc signatures.
    # Independently check every code-page hash and the bundle's resource seals.
    verify_binary(shell, app)
    verify_binary(backend)
    (stage/'使用说明.txt').write_text('PairShell '+args.version+'\n将 PairShell.app 复制到 Applications 或其他目录即可使用。\n支持 Apple Silicon 与 Intel Mac，在应用首页切换发起协助与接受协助。\n配置和历史记录保存在目标 Mac 当前用户的应用数据目录，本包不含任何机器的账号、服务器配置或证书。\n默认使用官方通道 https://relay.pairshell.top；可配置自定义服务器。\n应用采用 ad-hoc 签名，未做 Apple Developer ID 公证。\n',encoding='utf-8')
    with zipfile.ZipFile(output,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
        for path in sorted(stage.rglob('*')):
            if not path.is_file():continue
            entry=zipfile.ZipInfo(path.relative_to(stage).as_posix());entry.create_system=3
            mode=0o755 if path in [shell,backend] else 0o644
            entry.external_attr=(0o100000|mode)<<16;entry.compress_type=zipfile.ZIP_DEFLATED
            archive.writestr(entry,path.read_bytes())
    with zipfile.ZipFile(output) as archive:
        assert archive.testzip() is None
        assert archive.getinfo('PairShell.app/Contents/MacOS/PairShellCustomer').external_attr>>16&0o111
    print('UNIVERSAL_ZIP_READY',output,hashlib.sha256(output.read_bytes()).hexdigest())

if __name__=='__main__':main()
