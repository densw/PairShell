#!/bin/sh
# Run on macOS with two client app ZIPs; does not require Go on the Mac.
set -eu
if [ "$#" -ne 3 ]; then
  echo 'Usage: package-macos-customer.sh APPLE_SILICON_ZIP INTEL_ZIP OUTPUT_DMG' >&2
  exit 2
fi
arm_zip=$1
intel_zip=$2
output=$3
if [ -e "$output" ]; then echo 'Output already exists; refusing overwrite' >&2; exit 1; fi
work=$(mktemp -d "${TMPDIR:-/tmp}/pairshell-dmg.XXXXXXXX")
mkdir "$work/arm" "$work/intel" "$work/image"
ditto -xk "$arm_zip" "$work/arm"
ditto -xk "$intel_zip" "$work/intel"
app='PairShell 客户端.app'
ditto "$work/arm/$app" "$work/image/$app"
binary='Contents/MacOS/PairShellCustomer'
lipo -create "$work/arm/$app/$binary" "$work/intel/$app/$binary" -output "$work/image/PairShellCustomer.universal"
mv "$work/image/PairShellCustomer.universal" "$work/image/$app/$binary"
chmod 755 "$work/image/$app/$binary"
lipo "$work/image/$app/$binary" -verify_arch arm64 x86_64
plutil -lint "$work/image/$app/Contents/Info.plist"
# Ad-hoc signing establishes local code integrity, not Developer ID trust.
codesign --force --sign - --timestamp=none "$work/image/$app"
codesign --verify --deep --strict --verbose=2 "$work/image/$app"
ln -s /Applications "$work/image/Applications"
cat > "$work/image/安装说明.txt" <<'README'
PairShell 客户端 · Mac Universal 开发版

将「PairShell 客户端.app」拖到「Applications」，再从应用程序双击启动。
程序会自动打开本机客户页面。粘贴商家邀请、核对校验码，然后选择权限并确认授权。
客户全程不需要输入命令，也不需要安装 Go、Node 或 Python。
请保持客户控制页面开启；随时可以点击暂停或结束。

本包支持 Apple 芯片及 Intel Mac。已做本地 ad-hoc 签名，尚未使用 Apple Developer ID 签名或公证。
macOS 仍可能阻止首次启动。仅在确认来源可信后，使用系统设置中允许的方式打开；受管理设备可能禁止运行。
本包是开发测试版，完整安全、跨系统及网络验收尚未完成。
README
hdiutil create -volname 'PairShell 客户端' -srcfolder "$work/image" -format UDZO -fs HFS+ "$output"
hdiutil verify "$output"
shasum -a 256 "$output" > "$output.sha256"
echo "OUTPUT=$output"
echo "STAGING=$work"
