# 参与 PairShell 开发

PairShell 仍处于开发阶段。提交较大改动前，建议先开一个 issue 说明问题、预期行为和安全影响，避免重复工作。小型修复可直接提交 Pull Request，并说明用户能观察到的变化。

## 本地开发

- Go 1.27.4 或以上；桌面客户端使用仓库内嵌 UI 资源。
- 保持改动聚焦，遵循现有目录和代码风格；Go 代码使用 `gofmt`。
- 提交前运行 `go test ./...` 和 `go vet ./...`，并在 PR 描述中列出实际运行的检查。涉及 UI、打包或平台行为的改动，应说明验证的平台和边界。
- 修改配对、身份核验、授权、撤权、SSH 或审计逻辑时，补充覆盖成功与拒绝路径的回归测试；不要增加绕过客户授权或审计的执行路径。
- 新增依赖时说明用途、版本和许可；不要提交发布二进制、个人日志、密钥、凭据、`.env` 或 `.tools/` 构建工具。

## 构建与目录

- `cmd/`、`internal/`：Go 入口、实现和回归测试；`native/`：Windows / macOS 原生壳；`assets/`：当前资源；`deploy/`：服务部署；`scripts/`：构建及验证；`docs/`：当前文档；`dist/`：最新发布文件。
- 编译工具放在系统 PATH 或用户缓存，不存入项目。可用 `PAIRSHELL_BUILD_TOOLS` 指定缓存；Windows 默认 `%LOCALAPPDATA%/PairShell/BuildTools`，包含 Go、.NET 10 SDK、NSIS、7-Zip。已有工具无需重复下载。
- Windows / Linux / macOS Go 后端：运行 `./scripts/build.ps1`。中间二进制生成在忽略的 `bin/`，最终交付统一放入 `dist/`。
- Windows 原生包：依次运行 `./scripts/package-windows-window.ps1 -Version 1.1.3` 与 `./scripts/package-windows-installer.ps1 -Version 1.1.3`。
- macOS 通用包：在 Mac 上执行 `PAIRSHELL_VERSION=1.1.3 sh scripts/package-macos-window.sh bin/pairshell-darwin-arm64 bin/pairshell-darwin-amd64 native/macos/CustomerWindow.swift bin/PairShell-1.1.3-macOS-universal.dmg`。脚本从当前 Swift 源码构建双架构原生壳，同时生成同一签名 App 的 ZIP 与 DMG。
- 组装：`python scripts/assemble-release.py --version 1.1.3`，默认输出 `dist/`，也可用 `--output` 指定尚不存在的目录。发布目录只包含文件，不复制角色子目录或解压副本。验签见[发布签名说明](docs/release-signing.md)。
- Windows 固定路径测试：`./scripts/test-windows.ps1`；首次只编译可用 `-CompileOnly`。可选防火墙脚本 `scripts/allow-test-firewall.ps1` 只应由管理员按实际测试需要运行，不由普通构建自动调用。
- 测试和打包临时目录位于系统临时目录或用户工具缓存；`native/windows/bin`、`native/windows/obj`、`bin/` 均为可清理的生成物。

## 安全问题

不要在公开 issue 中贴出真实配对码、邀请、会话日志或凭据。发现可复现的安全问题时，先联系仓库维护者协调私下处理，并提供最小复现和受影响版本。

## 许可

仓库目前尚未发布 PairShell 自身的开源许可证。公开可见不等于允许任意复制、分发或商用；在许可证明确前，贡献是否可接受及其授权范围需由维护者逐项确认。
