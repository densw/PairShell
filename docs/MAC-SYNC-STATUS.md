# 同步交接记录 · 2026-11-16

Mac 源码：`/Users/macmini/Desktop/PairShell/Source-20261116-072103`。

包含：Go 模块源码与测试、Mac 原生窗口、Windows 原生窗口及锁定 NuGet 清单、共享 UI HTML/CSS、静态 UI 设计预览、Windows 窗口截图、打包脚本、技术规范、依赖许可证及开发文档。账号密码、.git、会话日志和临时身份不在源码归档中。

Mac 副本额外配置：`.tools/go` 为校验过 SHA-256 的 Go 1.27.4 arm64 工具链；`.tools/gomod` 为与 go.sum 固定版本匹配的离线模块缓存。优先使用 `sh scripts/mac-go.sh` 调用工具链。

最新交付：

- Mac：`PairShell-Customer-0.4.0-universal-window.dmg`
- Windows：`PairShell-Customer-0.4.0-Windows-x64.zip`
- 两份产物均在 Mac 桌面 `PairShell` 和原项目 `bin` 内。
- UI：双击 `docs/UI-PREVIEW.html` 查看无连接的设计稿；修改 `internal/uiassets` 即可修改真实应用共享界面。

验证：Windows/Mac 全量 Go 测试和 go vet 通过；Mac Swift 双架构构建、原生窗口启动/退出与 DMG 校验通过；Windows 自包含发布和真实 WebView2 加载/截图/退出 smoke 通过，NuGet 易受攻击包检查未报告问题。未完成正式平台签名、Apple 公证、完整远程协助 UI 矩阵与干净 Windows 安装测试；Windows 依赖系统 WebView2 Runtime。

Mac 模块初次网络下载失败已通过项目内离线缓存解决，未修改系统级 Go、代理或安全设置。旧版 DMG 保留以便对照，请以以上最新文件名为准。

0.4.0 UI 重新设计：深色紧凑工作台，共享 UI 源码、静态预览与真实窗口截图已更新。

0.5.0 最新双端：PairShell-0.5.0-macOS-universal.dmg / PairShell-0.5.0-Windows-x64.zip。源码含商家窗口、公网部署脚本及跨机器测试，见 HOSTHATCH-DEPLOYMENT.md 与 VERIFICATION-0.5.md。
