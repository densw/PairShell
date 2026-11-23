"""Package prebuilt Mach-O clients as Finder-extractable app ZIPs.

ZIP Unix modes preserve executable bits without chmod on the user's Mac.
Signing/notarization and DMG creation require the separate macOS release gate.
"""
import hashlib
import pathlib
import plistlib
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'bin'
APP = 'PairShell 客户端.app'
PLIST = plistlib.dumps({
    'CFBundleName': 'PairShell 客户端',
    'CFBundleDisplayName': 'PairShell 客户端',
    'CFBundleIdentifier': 'org.pairshell.customer',
    'CFBundleExecutable': 'PairShellCustomer',
    'CFBundlePackageType': 'APPL',
    'CFBundleShortVersionString': '0.1.0',
    'CFBundleVersion': '1',
    'LSUIElement': True,
    'LSMinimumSystemVersion': '13.0',
    'NSHighResolutionCapable': True,
})
README = '''PairShell 客户端（未签名开发版）

1. 双击 ZIP 解压，将「PairShell 客户端.app」拖入「应用程序」。
2. 双击应用，在自动打开的浏览器页面粘贴邀请，点击连接。
3. 核对校验码，打开客户控制页面选择权限并确认授权。
4. 协助中保持控制页面开启，结束时点击「结束」。

不需要终端、Go、Node 或 Python。需要可信 HTTPS 协调服务和商家端。
首次启动可能被 macOS 安全策略阻止：此包未经过 Apple 签名和公证。
只在确认来源可信时，通过系统允许的方式批准打开；受管理的 Mac 可能禁止运行。
当前仅完成交叉编译，尚未进行 Mac 真机验收。应用最低版本声明为 macOS 13，实际兼容性待验证。
日志可能含敏感信息，保存于 ~/Library/Application Support/PairShell/Sessions。
这是开发测试包，不是已完成签名公证的正式发行版。
'''.encode('utf-8')


def put(z, path, data, mode):
    i = zipfile.ZipInfo(path)
    i.create_system = 3
    i.external_attr = mode << 16
    i.compress_type = zipfile.ZIP_DEFLATED
    z.writestr(i, data)


for arch, label in [('arm64', 'AppleSilicon'), ('amd64', 'Intel')]:
    executable = (OUT / f'pairshell-darwin-{arch}').read_bytes()
    if executable[:4] != b'\xcf\xfa\xed\xfe':
        raise RuntimeError('Expected 64-bit Mach-O executable')
    name = OUT / f'PairShell-Customer-macOS-{label}-unsigned.zip'
    with zipfile.ZipFile(name, 'w') as z:
        for d in [APP, APP + '/Contents', APP + '/Contents/MacOS', APP + '/Contents/Resources']:
            put(z, d + '/', b'', 0o40755)
        put(z, APP + '/Contents/Info.plist', PLIST, 0o100644)
        put(z, APP + '/Contents/MacOS/PairShellCustomer', executable, 0o100755)
        put(z, '使用说明.txt', README, 0o100644)
        put(z, APP + '/Contents/Resources/THIRD_PARTY_NOTICES.md', (ROOT / 'docs/THIRD_PARTY_NOTICES.md').read_bytes(), 0o100644)
    with zipfile.ZipFile(name) as z:
        assert z.testzip() is None
        info = z.getinfo(APP + '/Contents/MacOS/PairShellCustomer')
        assert (info.external_attr >> 16) & 0o111 == 0o111
        assert plistlib.loads(z.read(APP + '/Contents/Info.plist'))['CFBundleExecutable'] == 'PairShellCustomer'
    name.with_suffix(name.suffix + '.sha256').write_text(hashlib.sha256(name.read_bytes()).hexdigest() + '  ' + name.name + '\n', encoding='utf-8')
    print(name)
