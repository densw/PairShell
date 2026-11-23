#!/bin/sh
set -eu
version=${PAIRSHELL_VERSION:-1.0.0}
# Inputs: arm64 Go executable, x86_64 Go executable, Swift source, output DMG.
test "$#" -eq 4
test ! -e "$4"
work=$(mktemp -d "${TMPDIR:-/tmp}/pairshell-window.XXXXXXXX")
app="$work/image/PairShell.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers" "$app/Contents/Resources"
cp "$(dirname "$0")/../docs/THIRD_PARTY_NOTICES.md" "$app/Contents/Resources/THIRD_PARTY_NOTICES.md"
brand="$(dirname "$0")/../assets/brand"
iconutil -c icns "$brand/PairShell.iconset" -o "$app/Contents/Resources/PairShell.icns"
lipo -create "$1" "$2" -output "$app/Contents/Helpers/PairShellBackend"
for arch in arm64 x86_64; do
  xcrun swiftc -swift-version 5 -target "$arch-apple-macosx13.0" -O -framework Cocoa -framework WebKit "$3" -o "$work/window-$arch"
done
lipo -create "$work/window-arm64" "$work/window-x86_64" -output "$app/Contents/MacOS/PairShellCustomer"
cat > "$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>
<key>CFBundleName</key><string>PairShell</string><key>CFBundleDisplayName</key><string>PairShell</string>
<key>CFBundleIdentifier</key><string>org.pairshell.desktop</string><key>CFBundleExecutable</key><string>PairShellCustomer</string>
<key>CFBundleIconFile</key><string>PairShell.icns</string>
<key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>1.0.0</string><key>CFBundleVersion</key><string>100</string>
<key>LSMinimumSystemVersion</key><string>13.0</string><key>NSHighResolutionCapable</key><true/>
<key>NSAppTransportSecurity</key><dict><key>NSAllowsLocalNetworking</key><true/></dict>
</dict></plist>
PLIST
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" "$app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c 'Set :CFBundleVersion 100' "$app/Contents/Info.plist"
chmod 755 "$app/Contents/MacOS/PairShellCustomer" "$app/Contents/Helpers/PairShellBackend"
codesign --force --sign - --timestamp=none "$app/Contents/Helpers/PairShellBackend"
codesign --force --sign - --timestamp=none "$app"
codesign --verify --deep --strict "$app"
lipo "$app/Contents/MacOS/PairShellCustomer" -verify_arch arm64 x86_64
ln -s /Applications "$work/image/Applications"
printf '%s\n' "PairShell $version 双端 · 独立终端客户端" '把 PairShell 拖入 Applications 后启动，在应用内切换发起协助或接受协助；客户分享配对码，商家连接后直接使用交互终端。' '关闭窗口或退出应用会结束协助。此开发包仅 ad-hoc 签名，未做 Apple 公证。' > "$work/image/安装说明.txt"
hdiutil create -volname 'PairShell 远程协助' -srcfolder "$work/image" -format UDZO -fs HFS+ "$4"
hdiutil verify "$4"
shasum -a 256 "$4" > "$4.sha256"
echo "APP=$app"
