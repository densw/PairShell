# PairShell 1.1.3

当前版本包含最新前端 UI、可读日志和时钟／租约修正。全系发布文件统一保存在项目 `dist/`，目录内只有文件。

## 更新

- 邀请、设备码交付和握手校验使用可信 HTTPS 协调服务时间，兼容精确时间头与 HTTP Date；计入网络延迟和秒级精度误差，避免机器时钟偏差延长授权。
- 租约使用本地单调计时，最多 30 秒；续租请求最多 10 秒且受现有截止点限制。缺失／无效／过期的数据撤销授权。时区仅影响显示。TLS 证书校验保持开启，系统日期超出证书有效期时仍需校时。
- 历史与活动日志过滤 ANSI／OSC／CSI 控制序列，跨事件和分页连续解码 UTF-8；合并终端输出、隐藏空白重绘，结束时读取剩余日志页。原始审计、哈希链、cast 与终端回放不变，旧记录无需迁移。
- 历史弹窗标题固定，列表与日志独立滚动，短屏下详情和状态保持可达。
- 可选 tailcat 传输通过 WireGuard / magicsock 支持直连与 DERP。两端设置 `PAIRSHELL_TRANSPORT=tailcat` 后协商启用；默认 WebRTC。详见[传输说明](transport.md#tailcat-backend)。

## 发布文件

| 平台 | 文件 |
| --- | --- |
| Windows x64 求助方 | `PairShell-1.1.3-Windows-x64-Customer-Setup.exe` |
| Windows x64 协助方 | `PairShell-1.1.3-Windows-x64-Merchant-Setup.exe` |
| Windows x64 双角色便携版 | `PairShell-1.1.3-Windows-x64.zip` |
| macOS Intel / Apple Silicon | `PairShell-1.1.3-macOS-universal.dmg` |
| Linux amd64 / arm64 CLI 与协调服务 | `PairShell-1.1.3-Linux-amd64.tar.gz`、`PairShell-1.1.3-Linux-arm64.tar.gz` |

同目录提供说明、构建校验报告、来源摘要、发布公钥及签名清单。安装包摘要以当前 `SHA256SUMS.txt` 为准，[验签方法](release-signing.md)。

## 验证与边界

Go 全量测试／vet、15 组界面测试、双角色 Windows / Mac 原生启动、安装包结构、卸载／路径临时夹具、正负 48 小时时间偏差与双传输 HTTPS／PAKE／SSH 回归、Mac 竞态检查和原生签名／DMG 完整性检查通过。第三方声明覆盖五个平台目标的 74 个运行时模块。

Windows 无 Authenticode 签名；Mac 为 ad-hoc 签名，未做 Developer ID 签名／公证。完整系统安装升级／卸载、Gatekeeper、跨机迁移及公网跨平台远程协助矩阵尚未完整验收。发布摘要签名不等同于平台信任认证。本次目录清理保留安装包二进制内容，只精简目录、工具和文档。
