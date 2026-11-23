# 客户端 UI 与独立窗口设计（Mac / Windows）

## 产品体验

桌面应用窗口不显示地址栏，不启动外部浏览器；Mac 使用系统 AppKit + WKWebView，Windows 使用 WinForms + 官方 WebView2。两端共享 `internal/uiassets` 的 HTML/CSS，授权与执行逻辑仍由 Go 后台处理。

## 视觉规范

0.4.0 已重做为桌面工作台：石墨底色 `#171b23`、面板 `#202630`、描边 `#343d4c`、克制蓝色主操作 `#426cd7`。顶部统一应用栏与真实状态，主标题 23px、正文 13px、辅助文本 10–12px，中文优先使用系统黑体。邀请页采用连接图/表单与安全说明双栏；控制页采用身份核验、权限清单与固定操作条。650px 以下单栏。取消宣传文案和大面积空白，所有权限仍默认关闭。

## 页面与状态

1. **接收邀请**：三步导航、说明、邀请输入、连接按钮。明确“连接不代表授权执行”；拒绝公开显示邀请内容。
2. **核对身份与授权**：校验码、连接详情、默认全部关闭的终端/任务/文件读写/AI 五个授权项。终端广泛权限及 AI 数据流向明确呈现。
3. **协助中**：真实状态徽标、暂停/恢复/结束、任务输出、审计事件。按钮按真实状态启禁。暂停只阻止新操作，结束不回滚修改。
4. **失败或结束**：保留原因和日志提示；不盲目重试已提交的有副作用任务。当前每次应用窗口绑定一次会话，重新协助需重新启动。

## 源码入口

- `internal/uiassets/invite.html`：邀请页
- `internal/uiassets/control.html`：核验、授权、控制页
- `internal/uiassets/ui.css`：共享样式和响应式规则
- `cmd/pairshell/desktop.go`：邀请页 HTTP 与窗口 IPC
- `internal/localui/server.go`：授权控制、状态与日志接口
- `native/macos/CustomerWindow.swift`：Mac 原生窗口
- `native/windows/Program.cs`：Windows 原生窗口

原生层只接受后台管道声明的回环 origin，禁止外站导航与弹窗。界面必须保持 Host/Origin/CSRF/会话授权检查，远端输出只通过 textContent 显示。不要让 UI 开关或原生壳成为绕过 Go 授权检查的第二条执行路径。

## Mac 接续开发

项目同步副本位于 Mac 桌面 `PairShell/Source-<同步时间>`，其中 `MAC-DEVELOPMENT.md` 是交接入口。同步包不含 `.git`、Windows 工具链、运行日志、账号密码或临时凭据。预览只需编辑共享 HTML/CSS 后重建 Go 后台；修改原生窗口才需要重编 Swift。源码副本与运行安装包分开放置。

Windows 新增 NuGet 依赖仅为官方 `Microsoft.Web.WebView2`，版本及传递依赖锁定在 csproj/packages.lock.json。Windows 包自带 .NET 运行时，WebView2 Runtime 单独依赖系统安装。构建期间的 UI 启动 smoke 不等价于完整双端协助验收。
