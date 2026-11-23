# PairShell 1.0.0

## 本次功能

- 默认勾选终端、命令任务、文件读取、文件修改和 AI（MCP）接入，主界面直接展示；客户核对身份后只需一次“允许完整协助”。配对本身不会自动授权。
- 桌面和 CLI 客户会话使用远程当前账号的真实文件系统。文件工具接受绝对路径；相对路径基于 `session_info.working_directory`。不再要求共享目录，也不再把任务工作目录限制在初始目录中。
- 任务和终端继承远程进程的环境变量；终端打开时也能使用 MCP 文件工具和命令任务。AI 不指定 Shell 时自动使用远程默认 Shell，不要求手动填写。
- 保留身份核对、能力选择、AI 数据去向说明、暂停、结束撤权、写入前内容摘要校验及本机审计。不自动提权，不改变操作系统 ACL、隐私权限或管理员授权。
- 文件接口每次读写仍有 1 MiB 限额，任务和传输仍有资源上限；大文件、目录管理及其他系统操作可通过已授权终端／任务完成。网络延迟、系统限制和所使用 AI 工具的能力仍可能影响体验，不能保证所有场景与本机完全一致。

## Windows 安装包

```text
PairShell-1.0.0-Windows-x64/
  PairShell.exe
  使用说明.txt
  runtime/
    PairShellBackend.exe
    licenses/
    （构建生成的其他组件）
```

采用 .NET 单文件自包含发布，把运行依赖合并入启动程序；后台程序、WebView2 原生加载器及第三方声明集中在 `runtime`。必须整体解压并保留该子目录，运行时不需要另装 .NET，但仍需要 WebView2 Runtime。CLI／MCP 程序入口为 `runtime/PairShellBackend.exe`。此发布方式使用微软的[单文件部署机制](https://learn.microsoft.com/en-us/dotnet/core/deploying/single-file/overview)，原生加载器通过[官方路径配置接口](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2environment.setloaderdllfolderpath)加载。

## 验证

- 固定路径全量 Go 测试、`go vet ./...` 通过。
- 12 组 Node 界面行为测试通过；授权模板验证全部能力默认可见并勾选，无共享目录输入或权限弹窗。
- Windows、Mac 的新完整协助测试各连续三次通过：授权前拒绝文件访问，授权后可读写工作目录以外的真实路径，终端保持打开时可同时读写文件和执行任务，任务使用实际工作目录和继承的环境；暂停／结束后拒绝新文件操作。
- Windows PE 产品版本为 `1.0.0`，后台与跨平台构建对应架构摘要一致；ZIP 结构与 CRC 检查记录于发布清单。
- 自动审批拦截了 Windows 新 EXE 的原生窗口启动测试，仅返回 `blocked by policy`，无详细原因。不能把编译与行为测试通过等同于该单文件包的原生窗口启动已通过。
- Mac DMG 由现有 Mac 在隔离的构建目录生成，不覆盖桌面现有安装；采用 ad-hoc 签名，尚未进行 Apple Developer ID 公证。

## 主要修改

`internal/executor/tasks.go`、`files.go`、`terminal.go`、`system_files_test.go`；`cmd/pairshell/main.go`；`internal/localui/bridge.go`；`internal/mcp/mcp.go`；`internal/uiassets/control.html`、`control.js`、`ui.css` 和授权模板测试；Windows 原生项目和两端打包脚本。

原有限目录执行器作为内部兼容模式保留，产品会话明确使用 `NewSystem`。新模式仍经过同一会话授权、撤销和审计路径。
