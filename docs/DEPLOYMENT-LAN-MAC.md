# 局域网 Mac 客户端部署

目标：局域网内的 Apple Silicon Mac，macOS 26.4。账号密码仅用于交互 SSH 连接，未写入项目、安装包或配置。

## 安装目标与构建

- 桌面现有文件夹：`~/Desktop/PairShell`。
- 客户端包：`PairShell 客户端.app`，独立 Cocoa 窗口，本地内嵌终端/控制资源，不需要浏览器。
- Swift 原生壳在目标 Mac 用 Command Line Tools 编译为 arm64；Go 后端使用本轮最终跨平台构建。
- 图标从原始 `assets/brand/PairShell.iconset` 经 macOS `iconutil` 生成 `.icns`，通过 `CFBundleIconFile` 加载。
- 应用采用本机 ad-hoc 签名；并非 Apple Developer ID 签名或公证发布包。

## 服务与证书

服务地址：`https://<LAN-IP>:9443`。

用户配置位置：`~/Library/Application Support/PairShell/config.json`。配置仅包含服务器地址和该服务的公开 CA PEM，权限为当前用户读写。额外证书信任仅作用于配置的完整服务来源；不修改系统钥匙串，不跳过 TLS、主机名或证书期限验证。

## 验证门禁

安装完成前检查：Go 后端与本机构建 SHA-256 一致、Mach-O arm64、`codesign --verify --deep --strict`、配置文件 0600、通过专用 CA 的 HTTPS 健康检查，以及应用自身 `--smoke-test` 的原生窗口加载结果。

## 已完成的安装与验证

- 已安装并打开：`~/Desktop/PairShell/PairShell 客户端.app`。
- 上一轮布局修复版后端传输前后 SHA-256 一致：`03f02ea6847ff150e8ed8b05033f1c6c32857c1aef13517f410b0b771da9ad61`。此构建保留同时直连/中继候选、测量最低 RTT 且不设 10ms/20% 优势门槛的修正。随后在 Mac 上按发布流程重新进行 ad-hoc 签名，签名后文件摘要不用于和原始传输摘要混淆。
- 原生程序与后端均为 Mach-O arm64；图标字段 `CFBundleIconFile` 为 `PairShell.icns`。
- 应用及后端完成 ad-hoc 签名，安装后 `codesign --verify --deep --strict` 通过。
- 应用原生窗口实际运行 `--smoke-test`，返回 `WINDOW_SMOKE_READY: native window + embedded customer page`。
- Mac 使用专用公开 CA 校验 LAN 服务，`https://<LAN-IP>:9443/healthz` 返回 `{"status":"ok"}`。
- 使用已安装的最终 Go 后端调用本机 `/listen` 与状态接口，成功从 LAN 服务取得 14 位设备配对码。验证只输出就绪状态及位数，没有输出配对码、窗口令牌或邀请秘密，也未授予远程执行权限。
- 配置文件权限为 0600；没有修改系统钥匙串或关闭 TLS 校验。
- 最后通过 LaunchServices 打开原生客户端，确认 `PairShellCustomer` 和其 `PairShellBackend` 进程实际运行。桌面原有其他内容未覆盖。
- 最终选路修正版本替换时，只按完整应用路径停止本轮启动的两个 PairShell 进程；未动其他应用、证书信任或已有配置。替换后重新签名、严格验证和原生 smoke 均通过，再次打开了客户窗口。

本轮未把 API 配对码生成测试解释为已经有人操作或授权了真实 Mac 会话。跨机器完整终端操作的验收由整体部署流程单独记录。

## Mac 说明栏竖排布局回归

用户反馈后，在真正的 Mac WKWebView 中新增仅由 `--layout-test` 启用的只读几何检查与可选 WKSnapshot 输出，不改变授权、配对或页面内容。

- 修复前，920×720 和 700×550 的说明正文宽度均为 `[23,23,23]`，文档高度 1688，原生测试明确返回 `WINDOW_LAYOUT_FAILED`。
- 修复后，920×720 的说明正文宽度为 `[302,302,302]`，文档高度 720，横向滚动宽度 920。
- 700×550 按响应式规则隐藏说明栏，横向滚动宽度 700，文档高度 583；内容可纵向正常滚动。
- 两个尺寸均返回 `WINDOW_LAYOUT_READY`。原生截图保存在工作区 `.tools/mac-layout-fixed-920.png` 和 `.tools/mac-layout-fixed-700.png`，数字结果在 `.tools/mac-layout-fixed.json`。
- 布局测试进程临时指向本机未监听端口，以免截图捕捉有效配对码；截图里的“生成失败”仅是该测试状态，不代表真实 LAN 服务故障。保存的用户服务器配置未改动。
- 已将固定布局后端和带只读测试开关的 Swift 原生壳更新到桌面应用，重新签名、严格校验和原生 smoke 通过，并以正常 LAN 配置重新打开客户端。

## 2026-11-20 同窗口重连修复部署

- 已更新同一桌面应用的 `Contents/Helpers/PairShellBackend`，保留 Swift 原生壳、图标、公开 CA 和用户服务器配置。
- 本轮跨平台构建与 Mac 暂存文件在签名前 SHA-256 均为 `12886d60b75e272c6aa83f7c283900b96f553a7ae69aabe3e3c7935e1181a15d`。替换后重新签名后端及应用，`codesign --verify --deep --strict` 通过，后端为 Mach-O arm64。
- 已安装后端以真实 LAN 配置在同一进程连续调用两轮 `/listen`，两轮均取得不同的 14 位设备配对码，随后 `/reset` 恢复 `CanGenerate`，正常退出码为 0。验证没有输出配对码或本地令牌，也没有自动允许远程执行授权。
- Mac 通过应用专用公开 CA 验证 LAN HTTPS 健康检查成功；配置权限仍为 0600。
- 原生 `--smoke-test` 返回 `WINDOW_SMOKE_READY`；本轮 WKWebView `--layout-test` 返回 `WINDOW_LAYOUT_READY`，920×720 说明正文宽度为 `[302,302,302]`、文档高度 720；700×550 隐藏说明栏，文档高度 629、可正常纵向滚动，两种尺寸均无横向溢出。
- 布局结果和原生截图位于 `.tools/mac-reconnect-layout.json`、`.tools/mac-reconnect-layout-920x720.png` 与 `.tools/mac-reconnect-layout-700x550.png`。截图测试临时指定本机未监听端口以隐藏有效配对码；其中错误提示不是正常 LAN 配置的结果。
- 最后以正常配置重新打开桌面应用，确认原生窗口与后端两个进程运行。没有修改系统钥匙串或放宽 TLS 验证。

跨两台机器的人工授权终端重连未由该 Mac 部署检查自动执行；同窗口两轮完整授权及真实 SSH 输出由项目集成回归单独验证。

## 原生 Shell 当前位置提示符回归

默认交互终端优先使用客户环境中有效的绝对路径 `SHELL`；Mac GUI 没有该变量时优先 `/bin/zsh`，再回退到 Bash、sh。已知 Shell 以登录模式加载现有用户配置，并提供带真实当前位置的默认提示符；没有改写用户的全局 profile。

2026-11-20 已在本机 Windows 通过终端包测试和 `go vet`；在目标 Mac 运行实际 arm64 PTY 回归，`TestDefaultShellSelection`、`TestInteractiveShellUsesNativePromptDefaults`、`TestPTYPromptTracksCurrentDirectory` 全部通过。目录测试在真实 PTY 中执行 `cd`，从后续原生提示符确认子目录名称，随后正常退出。证据 `.tools/shell-prompt-verification.json`。

## 终端工作区与客户只读镜像版本部署

2026-11-20 已将通过统一验证的最终后端更新至同一桌面应用。传输前后未签名文件 SHA-256 均为 `f35e9584f6c3be3849f1521613f59fe2ba9943f0d4226edb95a87c9bb8e32891`，文件为 Mach-O arm64。只停止完整应用路径下的原生与后端进程，保留 Swift 原生壳、图标、CA 与配置；替换后分别重新签名 helper 与应用，严格签名验证通过。

- 目标 Mac 的真实 PTY 镜像回归 `TestTerminalView` 连续三次通过，确认客户观察流与商家实际收到的输出字节一致，并验证审计失败不展示、缓存有界、分页与拷贝隔离。
- 原生 `--smoke-test` 返回 `WINDOW_SMOKE_READY`；原生 `--layout-test` 返回 `WINDOW_LAYOUT_READY`，920×720 无溢出，700×550 无横向溢出且可纵向滚动。
- 截图与布局 JSON：`.tools/mac-experience-layout-920x720.png`、`.tools/mac-experience-layout-700x550.png`、`.tools/mac-experience-layout.json`。隔离截图指定本机未监听端口，画面中的配置错误不是正常 LAN 启动状态。
- 使用真实配置和专用 CA 的 LAN HTTPS 健康检查成功；已安装 helper 同一进程两轮取得不同的 14 位配对码，重置后均恢复可生成状态，正常退出码为 0。没有输出码或本地令牌，没有自动授予远程执行权限。
- 最后已以正常配置重新打开应用，确认原生与 helper 两个进程运行、配置权限仍为 0600；再次严格签名验证通过。SSH 已关闭。

完整安装证据：`.tools/mac-experience-verification.json`。这些检查不冒充人工授权的 Windows→Mac 实际终端演示；真实 PTY 与客户镜像一致性由上述原生回归验证。
