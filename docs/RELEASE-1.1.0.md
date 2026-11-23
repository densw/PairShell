# PairShell 1.1.0

## 功能与交互

- 本次操作记录取消翻页按钮，固定高度区域内滚动查看。历史记录从两种角色首页进入，支持搜索、创建手动备注、修改标题和备注、移至回收站及恢复。原始会话审计记录仍只读，不允许把备注伪造成已执行操作。
- 历史输出滚动按需读取；商家端也在本机记录终端输出。原始客户侧审计仍为主要操作证据。历史查看不等同于哈希链验证，完整性校验仍使用 `pairshell verify`。
- 终端、命令任务、文件读写、AI 接入延续一次完整协助授权；不要求共享目录。客户可随时暂停、结束，实际操作权限取决于远程运行账号。
- 服务器设置只显示用户自定义地址，默认留空。自定义服务优先，官方服务可用时才作为配对建立前的兜底；勾选“强制只使用自定义服务器”后不兜底。不会向官方候选转发自建服务器的部署令牌，也不会在已建立会话中更换配对服务器。
- 官方服务当前未开放，发行包未内置官方地址。未配置时明确提示填写自定义地址。CLI 显式 `--server` 或环境变量指定地址时只使用该地址。

## 发布文件

- Windows 安装修订：分别提供 `PairShell-1.1.0-Windows-x64-Customer-Setup.exe`（客户端）和 `PairShell-1.1.0-Windows-x64-Merchant-Setup.exe`（商家端）。核心版本保持一致，各自只安装对应角色的入口，应用内仍能切换角色。安装目录、开始菜单和注册表卸载项按角色隔离，允许同时安装。安装后各自目录内生成 `Uninstall.exe`，卸载只删除该角色拥有的安装文件，保留当前用户配置及历史。
- 目录浏览修复：不再通过 `.onVerifyInstDir` 拒绝磁盘根目录或普通非空父目录；改在目录页完成后和写入前验证最终角色子目录。NSIS 的[浏览回调源码](https://github.com/NSIS-Dev/nsis/blob/v313/Source/exehead/Ui.c)确认旧回调会影响文件夹选择框的确定按钮。选择父目录时使用 NSIS 原生的 [InstallDir 自动补齐](https://nsis.sourceforge.io/Docs/Chapter4.html#installdir)；手动输入 `D:`／`D:\` 也补齐 `D:\PairShell`，实际安装到 `D:\PairShell\Customer` 或 `D:\PairShell\Merchant`。既有空目录可用，非空角色目录需要匹配本角色的安装标记。
- Windows 便携 ZIP：根目录只有 `PairShell.Customer.exe`、`PairShell.Merchant.exe`、`使用说明.txt` 三个文件以及 `runtime` 子目录。整体解压，勿单独移动入口 EXE。自包含 .NET，仍需 Microsoft WebView2 Runtime。
- macOS：`PairShell-1.1.0-macOS-universal.zip`，单个 `PairShell.app`，首页切换角色，支持 Intel 和 Apple Silicon，最低系统声明 macOS 13。复制到 Applications 或其他目录使用，包内不含任何机器的账号、配置、证书或操作记录。本次在 Windows 本地复用已有通用原生壳并更新后端，没有连接远程 Mac 打包。
- Linux：amd64／arm64 的 CLI 与服务端压缩包。
- 唯一发布目录：`bin/release-1.1.0`；摘要与验证范围见同目录清单。

## 验证与限制

- 全量固定路径 Go 测试、`go vet ./...`、13 组 Node 界面行为测试和界面对比度检查通过。新测试覆盖历史 CRUD 保留原始日志、路径穿越和来源／令牌拒绝、滚动读取短分块、读取失败停止自动重试、自定义／官方候选顺序及强制模式。
- Windows 产品版本 1.1.0，安装器可解包，包含卸载 EXE，内置后端与当前构建摘要一致；便携目录结构和 ZIP CRC 检查通过。
- 安装修订的回归检查覆盖两个安装器分别解包、单一角色入口、相同原生壳／后端摘要、卸载 EXE，以及目录校验回调阶段和根路径补齐代码。目录浏览、磁盘选择、安装与卸载仍需原生界面验收；这组检查不代表已经完成实际安装。
- Mac 两个可执行文件均包含 x86_64 和 arm64，ZIP 保留执行权限；已检查 Mach-O 代码页和资源封装摘要。采用 ad-hoc 签名，未做 Apple Developer ID 签名与公证。
- [rcodesign 上游曾报告其 verify 会拒绝刚签名的文件](https://github.com/indygreg/apple-platform-rs/issues/65)。本次 verify 提示自身可能误报，并拒绝空 CMS；因此使用独立摘要检查，不能将它等同于 Apple `codesign`／Gatekeeper 的真机验收。
- Windows 原生窗口启动此前被自动审批拒绝（`blocked by policy`，未返回详细原因）；本次未绕过限制。安装／卸载未在当前系统实际执行；Mac 新通用 ZIP 尚未真机启动或在第二台 Mac 验证迁移。官方服务不可用，兜底目前只有候选逻辑测试，没有真实官方服务端联调。
- 未覆盖桌面既有安装、远程服务器或 Mac 当前使用的应用；使用新版需安装新包。

## 构建与主要文件

`scripts/build.ps1` → `package-windows-window.ps1` → `package-windows-installer.ps1`；本地 Mac 包使用 `package-macos-universal.py` 与 `verify_macho_signature.py`；最后 `assemble-release.py` 汇总检查。

安装器工具为 [NSIS 3.13 官方 ZIP](https://sourceforge.net/projects/nsis/files/NSIS%203/3.13/nsis-3.13.zip/download)，SHA-256 `ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8`。没有新增应用运行依赖。

主要修改：`internal/history/*`、`cmd/pairshell/history*`、桌面与商家设置／会话生命周期、`internal/config/*`、`internal/uiassets/history.js` 和记录视图、`native/windows/Launcher.cs`、原生 Windows 路径配置及上述打包脚本。安装修订另涉及 `native/windows/installer.nsi`、`scripts/test-windows-installers.py`、`scripts/assemble-release.py`。
