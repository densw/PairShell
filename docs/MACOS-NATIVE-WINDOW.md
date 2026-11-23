# Mac 客户独立窗口（0.2.0）

产物：`bin/PairShell-Customer-0.2.0-universal-window.dmg`。
Mac 副本：桌面 `PairShell` 文件夹同名 DMG。

双击 DMG，将应用拖入 Applications 并替换旧版。先退出旧版后再打开新版。新版通过 AppKit 原生窗口内嵌系统 WKWebView 显示客户界面，不调用默认浏览器；无地址栏，有 Dock 图标与退出/编辑菜单。

Swift 外壳启动包内 Go 后台，通过匿名管道接收一次性本机页面地址。仅允许后台声明的 127.0.0.1 HTTP origin 导航，使用非持久 WebKit 数据存储；邀请与控制仍经过既有 Host/Origin/令牌/CSRF 保护。关闭窗口或 Cmd+Q 发送 SIGTERM 给后台，等待会话清理；授权、执行与审计边界保持在 Go 客户端。

源码：`native/macos/CustomerWindow.swift`；打包：`scripts/package-macos-window.sh`。脚本编译 macOS 13+ 的 arm64/x86_64 原生外壳，合并两架构 Go 后台，签名内部 helper 与应用，再生成 DMG。无新增 Go 依赖。

2026-11-16 验证：

- Windows 上 `go test ./... -timeout=60s` 与客户入口 `go vet` 通过。
- Mac mini macOS 26.4：Swift 原生双架构编译、codesign 深度严格校验与 hdiutil 映像校验通过。
- 真机运行原生程序 `--smoke-test`：应用窗口内 WKWebView 成功加载客户入口，读取页面标题后正常退出并收尾后台，输出 `WINDOW_SMOKE_READY`。
- DMG 下载到项目后与 Mac 副本 SHA-256 一致。

尚未测试 Intel 真机、完整远程协助的原生窗口流程及异常退出矩阵。签名仍为 ad-hoc，没有 Apple Developer ID 或公证。旧浏览器版 0.1.0 产物保留，不要混用。
