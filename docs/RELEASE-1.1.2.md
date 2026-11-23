# PairShell 1.1.2

基于当前仓库最新代码构建，保留既有官方通道、自定义服务器及双端授权流程。本次没有覆盖现有安装或部署远程服务器。

## 界面修复

- 修复商家端服务器配置中复选框占满一行、标签被挤到最右侧并换行的问题。文本框样式明确排除复选框／单选框；两端服务器选项采用 18px 控件加弹性标签布局，整行可点击。
- 统一隐藏页面、配置／邀请／工具弹窗、历史列表／日志、文本域及终端的滚动条轨道、箭头和滑块，保留滚轮、触控板及键盘滚动。
- 历史记录没有选中条目时显示操作提示，隐藏空白编辑区和输出区；选中后才显示详情，删除或筛选掉选中条目时返回空状态。

主要界面文件：`internal/uiassets/ui.css`、`terminal-assets/workspace.css`、`invite.html`、`merchant.html`、`history.js`。版本号更新到 1.1.2，界面行为继续使用现有实现，没有新增应用运行依赖。

## 本地发布包

- Windows 客户端：`Windows-Customer/PairShell-Setup.exe`，默认接受协助。
- Windows 商家端：`Windows-Merchant/PairShell-Setup.exe`，默认发起协助。两个文件名相同，在分发目录中区分；安装后入口及快捷方式统一为 `PairShell`，主目录为 `PairShell.exe`、`Uninstall.exe`、使用说明及 runtime。应用内仍可切换角色。
- 两个安装包是同一个应用，仅默认入口角色不同。同一路径安装另一包会更新默认角色，使用同一套安装／卸载记录，不再追加 Customer／Merchant 安装子目录。旧角色文件名的安装 EXE 保留为内容一致的兼容下载副本。
- 文件夹选择器选中 `D:\Tools\` 后，返回路径页立即显示 `D:\Tools\PairShell\`；已选择 PairShell 文件夹时不重复追加。修复安装阶段对未创建目录的误判：NSIS 的 [GetFullPathName 指令](https://nsis.sourceforge.io/Reference/GetFullPathName)会对不存在的路径报错，改用不要求目标已存在的 Windows 路径规范化 API，保证重复检查结果一致。
- Mac：`PairShell-1.1.2-macOS-universal.dmg`，真实 UDIF／HFS+ 磁盘镜像，内含一个 `PairShell.app` 和 `/Applications` 拖拽安装链接。支持 Intel 与 Apple Silicon，配置和记录由目标 Mac 当前用户独立保存。ZIP 仅为构建中间文件，发行目录交付 DMG。
- Windows 便携包及 Linux CLI／服务端包保留在 `bin/release-1.1.2`。

Windows 便携包也已补入独立 `Uninstall.exe`，与两个角色入口、使用说明并列；运行组件及便携身份标记仍放在 `runtime`。卸载前会检查所在目录的标记和程序占用情况，仅删除包内明确列出的程序文件，保留配置、历史及额外文件，不修改安装版的注册表和快捷方式。安装版仍使用按角色生成的卸载入口。

Mac 构建使用已有通用原生壳和本次新编译后端，重新生成 ad-hoc 签名；本次没有连接远程 Mac。DMG 使用开源 [leaanthony/dmg](https://github.com/leaanthony/dmg) 的 HFS+／UDIF 写入器，固定构建版本 `v0.0.0-20260731075144-3d3581c3dfa3`。该库仅位于 `.tools/dmg-builder` 的独立构建模块，产品 `go.mod`／`go.sum` 不变。构建脚本明确保留两个 Mach-O 文件的 0755 权限、其他文件的 0644 权限及 Unix Applications 链接，避免 Windows 权限表示影响 Mac 包。

构建入口：`scripts/package-macos-dmg.ps1`；镜像写入入口：`scripts/dmg-builder.go`；检查：`scripts/test-macos-dmg.py`。原有通用 App 组装／签名步骤保持复用。

## 验证范围

- 全量固定路径 Go 测试、`go vet ./...`、14 组 Node 界面行为／浏览器布局测试、对比度检查和 5 组发布签名测试通过。
- 在独立 Windows 原生测试窗口观察了两端首页、修复后的服务器弹窗、历史记录空状态。配置值没有在此次检查中保存或更改；没有建立真实远程协助。视觉对比从截图所示错位改为控件与标签紧邻的一行布局。
- 两种 Windows 安装器编译／解包通过，仅包含对应入口；共用核心和后台摘要与当前构建一致，包含卸载 EXE。未执行实际安装／卸载。
- 安装修订回归：两包均含同名 PairShell.exe，反射调用编译后的纯默认角色函数，分别返回 customer／merchant。以实际 NSIS 路径函数验证父目录、现成后缀、根目录、缺失的多级中文目录及连续检查两次，另在 .tools 临时目录完成实际目录创建与文件写入；不写入系统安装记录或快捷方式。完整系统安装／卸载仍未由代理执行。
- 便携卸载程序已在 `.tools` 自建临时目录实际执行：错误身份标记被拒绝，正确标记的模拟包文件被移除，额外根目录和 runtime 内文件均保留。没有运行真实安装目录的卸载程序。生成入口为 `scripts/package-windows-portable-uninstaller.ps1`，回归检查为 `scripts/test-portable-uninstaller.py`。
- 7-Zip 独立检查确认 DMG 为 UDIF ZLIB、内层 HFS+，CRC 正确，包含 Applications 符号链接和两个可执行权限正确的 Mac 二进制文件。解包 App 的全部文件摘要与组装后的签名 App 一致，两个架构的代码页及资源签名摘要检查通过。
- Mac 尚未真机挂载、启动或迁移验收。包使用 ad-hoc 签名，未做 Apple Developer ID 公证；不能把结构／摘要检查等同于 Gatekeeper 或 Apple 真机验证。
- 本地包尚未上传为 GitHub 正式 Release。此次不更改现有 v1.1.1 的公网下载链接。

## 本次滚动与默认配置优化

- 新用户自定义服务器留空；默认通过官方通道连接。清除未明确保存的旧 LAN 测试地址，覆盖尾部斜杠变体；用户明确保存的自定义设置保持不变。先前 Mac LAN 测试部署曾保存用户配置，更新安装包不会自然清除用户目录。此次没有连接真实 Mac 读取其当前设置。
- 两端完整 24 位校验码单行显示，按容器宽度调整字号，不缩短、不截断校验码。极窄客户端窗口的授权面板改为单列并允许滚动。
- 浏览器实际检查商家端 640／850／1298／1587px、客户端 320／640／850／1587px；历史弹窗、列表和日志内容隐藏滚动条后仍能滚动。
- 最终 `bin/release-1.1.2` 只交付两个角色的 Windows 安装 EXE、一个 Mac 通用 DMG、说明及校验／签名文件。便携 ZIP、Linux 包与构建中间文件不放入该交付目录。
- 安装文件摘要清单使用现有独立 Ed25519 发布密钥生成 OpenSSH 分离签名。Windows 安装器没有 Authenticode 签名；Mac App 为 ad-hoc 签名，未做 Apple Developer ID 签名／公证。不能将发布清单签名等同于平台信任认证。
