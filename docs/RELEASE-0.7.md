# PairShell 0.7 桌面交互版

## 安装

Mac：打开 `PairShell-0.7.0-macOS-universal.dmg`，将「PairShell 商家端.app」和／或「PairShell 客户端.app」拖入 Applications，再启动所需角色。安装包同时含 arm64 与 x86_64；最低 macOS 13。配置位于用户 Application Support 中，安装包不携带账号密码或私有证书。

Windows：解压 `PairShell-0.7.0-Windows-x64.zip`，保留同目录文件，打开 `PairShell.Merchant.exe` 或 `PairShell.Customer.exe`。需要 WebView2 Runtime。

本次提供安装包供用户安装，没有替换现有 Mac 桌面应用。Mac 两个应用采用 ad-hoc 开发签名，严格签名检查通过，但尚未有 Developer ID／Apple 公证。

## 交互变化

- 客户：醒目的配对码与复制按钮，设置按需展开，保存服务器后自动重新生成配对码。
- 授权：先核对对象、再选能力；授权后的连接详情仍可从操作记录查看。
- 商家：连接前提供明确空状态，立即提交反馈、防止重复 Enter，失败保留重试入口；真正就绪后终端占满窗口。
- 操作：头部集中连接与链路延迟，客户侧实时只读观看，暂停／结束入口清晰；高级 CLI／MCP 保留。
- 一致性：系统字体、离线资源、统一间距／控件／焦点，主按钮正常文本对比度 4.95:1，无新依赖。

## 验证

- 固定路径全量 Go 测试、`go vet ./...`、五个平台构建通过。
- 重连、商家收栏／RTT 分级、客户镜像与结束尾帧、提交防重、保存配置自动更新配对码五组 Node 行为测试通过。
- 正文、次要文字、错误与主按钮的对比度检查通过。
- Windows 打包目录中的商家端、客户端均返回 `WINDOW_SMOKE_READY`。
- 从最终 DMG 只读挂载后，两个应用的原生壳与后端均通过 Universal 架构检查、严格签名检查；两个角色原生启动通过。目标 Mac 为 Apple Silicon，未在 Intel 真机运行。
- Mac WKWebView 920×720、700×550 的真实布局检查通过，无横向溢出，较长页面可纵向滚动。
- DMG `hdiutil verify` 通过；本机与 Mac 的安装包 SHA-256 一致。校验文件为 `PairShell-0.7.0-SHA256SUMS.txt`。

`docs/ui-0.7/` 包含关键状态及原生窗口截图。交互状态截图使用无远程执行能力的隔离示例，Mac 原生布局检查使用隔离测试地址，其加载或错误状态不代表正常服务器状态。组件、布局与构建检查不等价于完整跨网生产验收。
