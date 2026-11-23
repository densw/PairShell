# 在 Mac 上继续开发

从桌面 PairShell 中最新 Source-* 目录打开项目。UI 设计见 `docs/CUSTOMER-UI-DESIGN.md`；共享界面位于 `internal/uiassets`，原生 Mac 窗口位于 `native/macos/CustomerWindow.swift`，Windows 窗口位于 `native/windows`。

本次同步副本额外配置了项目内 Go：`.tools/go/bin/go`。在项目根目录执行 `export PATH="$PWD/.tools/go/bin:$PATH"` 后即可使用以下命令，不必安装系统级 Go。独立 UI 视觉预览可直接双击 `docs/UI-PREVIEW.html`，它不会连接真实客户；`docs/customer-ui-preview.png` 为 Windows 原生窗口实测截图。

因为该 Mac 无法访问默认 Go 模块代理，本次另外同步了固定依赖缓存到 `.tools/gomod`。推荐使用 `sh scripts/mac-go.sh test ./...`、`sh scripts/mac-go.sh vet ./...` 和 `sh scripts/mac-go.sh build ...`，无需联网下载现有依赖。依赖版本仍由 go.mod/go.sum 固定；有意新增依赖时再显式选择可达的模块源。

## 构建

需要 Go 1.26+ 和 Xcode Command Line Tools（当前 Mac 已验证 Swift 可用）。Go 未包含在源码归档中；如有 Go，先运行 `go test ./...` 与 `go vet ./...`。用户运行成品不需要安装这些开发工具。

```sh
mkdir -p bin
GOOS=darwin GOARCH=arm64 go build -trimpath -o bin/pairshell-darwin-arm64 ./cmd/pairshell
GOOS=darwin GOARCH=amd64 go build -trimpath -o bin/pairshell-darwin-amd64 ./cmd/pairshell
cp docs/THIRD_PARTY_NOTICES.md native/macos/THIRD_PARTY_NOTICES.md
sh scripts/package-macos-window.sh bin/pairshell-darwin-arm64 bin/pairshell-darwin-amd64 native/macos/CustomerWindow.swift "$HOME/Desktop/PairShell/PairShell-Customer-new-window.dmg"
```

脚本拒绝覆盖现有输出；请使用新文件名。默认 Swift 入口版本和 UI 源码是版本来源，正式发布前同步更新 Info.plist 版本号。

## 验证与接续事项

- `go test ./...` / `go vet ./...`；原生窗口 `--smoke-test` 会验证 WKWebView 加载后退出。
- Windows 构建：PowerShell 执行 `scripts/package-windows-window.ps1`，需要 .NET 9 SDK；用户成品不需要安装 .NET SDK。
- 原生窗口加载、授权页切换、长时后台心跳、窗口关闭撤销、断网与大输出需要持续真机验证。
- 当前是 ad-hoc 开发签名，未做 Developer ID、公证、Windows Authenticode。
- 完整工程缺口见 `docs/IMPLEMENTATION.md`。不要把独立窗口交付视为完整产品已通过安全和网络验收。
