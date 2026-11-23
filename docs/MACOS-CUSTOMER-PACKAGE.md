# Mac 客户安装包

文件：`bin/PairShell-Customer-0.1.0-universal-adhoc.dmg`。

客户操作：双击 DMG，将「PairShell 客户端.app」拖入 Applications，再从应用程序启动。默认进入客户浏览器界面，粘贴邀请、核对校验码并授权；不需要命令行。

2026-11-16 在局域网 Mac mini（macOS 26.4、Apple 芯片）制作 Universal DMG，含 arm64 与 x86_64。Mac 桌面 `PairShell` 文件夹保留相同产物，项目 bin 目录及 Mac 均附 SHA-256。

产物使用 ad-hoc 本地签名，`codesign --verify --deep --strict` 与 `hdiutil verify` 已通过。没有 Developer ID 签名或 Apple 公证，系统可能阻止首次启动。项目副本下载后与 Mac 文件哈希一致：

`05a83716469e28ea96f385db28aefd74808706ef14499a14f6cc454ab41ac940`

复现脚本：`scripts/package-macos-customer.sh`，接受 Apple 芯片 ZIP、Intel ZIP、DMG 输出路径三个参数。脚本拒绝覆盖已有 DMG，使用临时目录打包，不修改系统安全设置。

这是开发测试包，打包与签名验证不替代端到端维修会话、Intel 真机及安全发布验收。
