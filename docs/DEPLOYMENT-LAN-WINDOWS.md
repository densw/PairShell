# 本机 Windows 商家端部署

目标：当前 Windows 用户桌面的 `PairShell-商家端` 目录，桌面入口为 `PairShell 商家端.lnk`。安装目录包含完整桌面运行文件；用户通过快捷方式进入商家终端，不需要浏览器。

客户端使用 `https://<LAN-IP>:9443`。配置位于 `%APPDATA%\PairShell\config.json`，由 `config.SaveDefault` 原子写入并设置当前用户专属 ACL。配置包含本服务的公开 CA 证书，额外信任只对这个服务地址生效；未关闭 TLS 校验，也未修改 Windows 全局证书信任。

本次 Logo 使用 `assets/brand/pairshell.svg` 原创母版，蓝色和青色配对括号包围终端提示符。多尺寸 ICO 已写入 PE 资源，WinForms 标题栏与任务栏从当前可执行文件提取同一图标，快捷方式也使用该图标。

2026-11-19 部署完成。桌面程序目录为 `%USERPROFILE%\Desktop\PairShell-商家端`，入口快捷方式为 `%USERPROFILE%\Desktop\PairShell 商家端.lnk`。目录含完整自包含运行文件，不应单独移动或删除 DLL 与后端程序。

验证：

- `dotnet publish` 成功，应用 PE 图标提取确认32×32自定义资源。
- 安装后的后端 SHA-256 与最终验证的 `bin/pairshell-windows-amd64.exe` 完全一致。
- 从桌面安装目录启动真实 WinForms/WebView2 商家窗口，`--smoke-test` 输出 `WINDOW_SMOKE_READY`，并保存界面截图；已核对内嵌新 Logo、主终端、配对入口和线路状态。
- 本机配置 ACL 禁止继承、所有者为当前用户；经应用 HTTP 客户端使用专用 CA 验证 `https://<LAN-IP>:9443/healthz` 返回200。
- Windows ZIP 全量 CRC 检查通过，最后正常打开商家窗口供用户使用。

截图：`.tools/windows-lan-installed-smoke.png`；图标提取：`.tools/windows-installed-icon.png`。测试工具文件不进入发布包。

限制：窗口启动、TLS 和安装完整性已验证；本条记录不替代 Mac→Windows 的实际跨机授权、交互与中继矩阵验收。Mac 客户端与 Docker 服务端的部署记录分别见对应文档。

## 最终构建刷新

已更新到移除选路绝对 RTT 门槛后的最终后端；只关闭本次部署打开的、路径核对一致的商家进程后替换。图标、桌面快捷方式和用户 CA 配置均保持不变。

- 后端 SHA-256：`bada43ca71fe061c99935a6f9f61c4b3a36313e5333c1c13c606007e4d45d071`
- Windows ZIP SHA-256：`3452bbf07f375b25911bfcc36b061ca1ce8be6d4102522af1ed37688260f5a9c`
- 包内后端、安装目录后端均与最终验证的架构二进制一致；ZIP CRC 通过。
- 新安装原生窗口最终 smoke 通过：`.tools/windows-lan-installed-final-smoke.png`，之后已正常重新打开商家窗口。

## 同窗口重新连接修复版

2026-11-20 已重新构建、更新桌面安装目录并打开商家窗口。安装后端与打包后端 SHA-256 一致：`c49b2303675f0b7b0888623daa57ba6d9a88e087f614b8c752995dc379393794`；本次 ZIP 摘要为 `5041a07de1ba3258e5b6ae396613b80bbfe2bc96be1fb50fd26a485162cbb5db`。

安装版原生 smoke 返回 `WINDOW_SMOKE_READY`，截图 `.tools/windows-reconnect-smoke.png`。实际窗口检查无效码失败后保留输入、显示就近错误、恢复“重新连接”，同窗口第二次 Enter 可提交。验证后已重新打开干净窗口。会话和自动化验证详见 `RECONNECT-VERIFICATION.md`。

## 终端专注与客户观看版

2026-11-20 再次更新同一桌面安装目录，原生 smoke 返回 `WINDOW_SMOKE_READY`，并重新打开商家端。安装后端与本次构建摘要一致：`3df868833ce7a56685f27f12fbcc8634ebe39e5ef5d1ec33a420f85a5fb80a9f`；ZIP 摘要 `0bc9ffe75d2a6b806f93e5f4a89a5348c4e3b73342377ee9c67e8078f1bf336a`。

新构建包含连接后自动收栏、头部线路／RTT 分级、客户只读终端、默认原生 Shell 目录提示符及结束尾帧同步。截图与摘要证据位于 `.tools/windows-experience-smoke.png`、`.tools/windows-experience-verification.json`。未修改用户服务器、CA 或已有精确路径防火墙规则。功能验证见 `TERMINAL-EXPERIENCE.md`。
