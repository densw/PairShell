# Windows 客户独立窗口

开发构建：`bin/PairShell-Customer-0.3.0-Windows-x64.zip`。解压到独立文件夹后，双击 `PairShell.Customer.exe`；保留同目录全部文件。窗口内完成邀请与授权，不打开 Edge/Chrome 浏览器窗口，不显示终端。

使用 WinForms + 微软官方 WebView2，包内包含 .NET 运行时；需要系统具备 Edge WebView2 Runtime。窗口仅接受 Go 后台声明的回环 origin，禁止新窗口、下载与 WebView 权限请求。关闭窗口先关闭后台 stdin，通过 EOF 请求撤销与清理，等待最多 12 秒后才进行进程树终止回退。该回退仍需加强不可纳管进程报告。

构建：`scripts/package-windows-window.ps1`。新增依赖与版本见 `native/windows/PairShell.Customer.csproj` 和 `packages.lock.json`，许可见 `WEBVIEW2-LICENSE.txt` / `WEBVIEW2-NOTICE.txt`，运行时许可包含在发布目录。

验证：Windows 本机独立窗口加载共享 UI、WebView 截图、窗口退出及后台正常退出 smoke 通过。Go 全量测试与 vet 通过；NuGet 检查未报告已知易受攻击包。尚未签名，未制作安装向导，未覆盖干净 Windows 系统、缺少 WebView2 Runtime 或完整远程协助矩阵。
