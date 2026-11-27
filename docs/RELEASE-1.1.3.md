# PairShell 1.1.3

发布日期：2026-11-27。以 Mac mini 桌面最新代码快照为基础，全平台统一更新为 1.1.3。本次重新构建客户端及协调服务发行文件，不自动安装客户端或升级已部署服务器。

## 本次更新

- 新增可选 tailcat 传输后端，通过 WireGuard / magicsock 建立连接，支持 UDP 直连和 DERP 中继。两端设置 `PAIRSHELL_TRANSPORT=tailcat` 后协商启用；默认仍使用 WebRTC。可用 `PAIRSHELL_DERP_MAP_URL` 配置 DERP，引导和安全边界见 [传输说明](transport.md#tailcat-backend)。
- 同步桌面代码包中的传输抽象、依赖锁定和相关测试；补全五个平台目标共 74 个运行时依赖模块的第三方许可声明。
- CLI、Windows 原生程序及启动入口、macOS App 版本统一为 1.1.3，打包和验包脚本默认版本同步更新。
- Linux CLI 与协调服务提供 amd64 和 arm64 两种架构，同步重新编译。

## 发行文件

- `PairShell-1.1.3-Windows-x64-Customer-Setup.exe`：默认接受协助。
- `PairShell-1.1.3-Windows-x64-Merchant-Setup.exe`：默认发起协助。
- `PairShell-1.1.3-Windows-x64.zip`：双角色便携版，包含独立卸载程序。
- `PairShell-1.1.3-macOS-universal.dmg`：Intel / Apple Silicon 通用版，单 App 内切换角色。
- `PairShell-1.1.3-Linux-amd64.tar.gz` 和 `PairShell-1.1.3-Linux-arm64.tar.gz`：各含 `pairshell`、`pairshelld` 及第三方声明。

Windows 两种安装包安装同一个应用，区别为默认入口角色；应用内可切换角色。macOS 在 Mac mini 从本次源码重新编译两个架构的 Swift 原生壳及 Go 后端，使用 ad-hoc 签名和原生 hdiutil 生成 DMG。

## 验证与限制

- Go 全量测试和 `go vet ./...` 通过；Windows、Linux、macOS 目标构建成功。
- 14 组 Node 界面行为／浏览器布局测试、对比度检查及 5 组发布签名回归通过。
- Windows 两种角色原生窗口加载通过；安装包解包、默认角色、共享后端摘要、便携卸载及安装路径回归通过。卸载和路径测试仅操作独立临时夹具，未改动现有安装。
- Mac mini 两种角色原生窗口启动通过；通用 Mach-O 架构、App 严格签名及 DMG 完整性检查通过。
- 发布清单使用既有独立 Ed25519 发布密钥生成 OpenSSH 分离签名；校验方法见 [发布签名说明](release-signing.md)。
- Windows 安装器没有 Authenticode 签名；macOS 未做 Apple Developer ID 签名／公证。未验证 Gatekeeper、完整 Windows 安装升级／卸载、跨机迁移及公网跨平台远程协助矩阵。
- 本次以桌面代码包为准；包内不含此前本地 1.1.3 的日志上传、时钟修正和诊断扩展，它们未并入本次发行。此前工作区已单独备份。
