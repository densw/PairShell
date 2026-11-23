# PairShell

## 三方安全协助

客户和商家通过彼此已经信任的渠道核对本次校验码。双方确认后，客户在自己的设备上决定是否授权。协调服务负责配对和转发，握手和 SSH 会话绑定到两端本次使用的密钥。具备服务端管理权限的人也必须通过同样的端到端身份核验，才能以协助方身份建立会话。

完整协助使用客户当前系统账号的权限，可操作该账号能访问的真实路径。只有确认对方是你信任的人后才允许。公网运营、服务器失陷、恶意对端和安装包来源的保护边界见 [三方安全协助说明](docs/trust.md)。强制双向校验适用于包含此改动的构建，请同时更新两端。

## 1.1.1：三方安全加固

会话校验码由双方用仅两端持有的邀请秘密计算。配对码生成端独立限制每个码最多验证三次，两端需一起升级。现有权限范围和默认选项保留。

终端、命令任务、文件读写与 AI（MCP）默认选中，直接在授权页显示，不再需要共享目录或多层权限弹窗。客户核对身份后允许一次，工具按远程当前账号的系统权限操作真实路径；暂停、结束撤权和审计保留，不自动提权。

- Windows 独立安装向导：[客户端 EXE](bin/release-1.1.1-official/PairShell-1.1.1-Windows-x64-Customer-Setup.exe)／[商家端 EXE](bin/release-1.1.1-official/PairShell-1.1.1-Windows-x64-Merchant-Setup.exe)。可同时安装，各自卸载；选择 `D:\` 自动补齐 `PairShell`，程序分别放入 `Customer`／`Merchant` 子目录。卸载保留配置及历史。
- [Windows 便携 ZIP](bin/release-1.1.1-official/PairShell-1.1.1-Windows-x64.zip)：根目录两个角色入口、使用说明，组件集中在 `runtime`。
- [macOS 通用 ZIP](bin/release-1.1.1-official/PairShell-1.1.1-macOS-universal.zip)：一个 PairShell.app，首页切换角色，兼容 Apple Silicon 和 Intel；ad-hoc 签名，尚未 Apple 公证。
- Linux [amd64](bin/release-1.1.1-official/PairShell-1.1.1-Linux-amd64.tar.gz)／[arm64](bin/release-1.1.1-official/PairShell-1.1.1-Linux-arm64.tar.gz) CLI 与服务端包。
- [发布签名验证](docs/release-signing.md) · [SHA-256 清单](bin/release-1.1.1-official/SHA256SUMS.txt) · [分离签名](bin/release-1.1.1-official/SHA256SUMS.txt.sig)。原生启动及安装卸载真机测试仍待完成。[包验证记录](bin/release-1.1.1-official/verification.json)

操作记录改为滚动查看，历史记录支持搜索、手动备注、标题／备注编辑和回收站恢复；原始审计日志保持只读。官方默认通道为 `https://relay.pairshell.top`。目前公网入口尚未开通。现有自定义地址优先，强制自定义模式不使用官方候选。

完整能力和验收边界见 [功能清单](docs/FEATURES-AND-UX.md)，设计规则见 [桌面设计规范](design-system/pairshell/MASTER.md)。

## 独立桌面终端客户端

Windows 双击 `PairShell.Customer.exe` / `PairShell.Merchant.exe`，所有操作在独立客户端窗口完成，无需打开浏览器。终端为离线内嵌 xterm.js，持续连接同一个 SSH PTY，不按命令分割、不逐条弹窗确认。

1. 客户打开客户端，自动生成五分钟有效的一次性配对码。
2. 商家在自己的客户端输入该码。
3. 双方通过已有的电话、聊天或当面互相核对本次校验码。各自在 PairShell 输入对方读取的校验码，客户再决定是否允许协助。
4. 客户可随时暂停新输入或结束；完整协助能力默认选中，日志始终开启。

官方通道开通前，请在“连接设置”中填写自建地址，CLI 也支持 `--server` 和 `PAIRSHELL_SERVER`。短码连接需要同步升级协调服务和两端客户端，客户授权后仍可使用 TURN 中继。会话默认 10 分钟无有效操作自动结束。部署限流和地址公开范围见 [部署说明](docs/deployment.md)。

CLI 同样支持：

```powershell
pairshell host                         # 客户出码，在命令行核验并授权
pairshell connect                      # 商家输入配对码，双方校验和客户授权后进入终端
pairshell connect --headless           # 完成相同校验后开放适配器，供 MCP 使用
pairshell shell --session <本地引用>     # 附加原生命令行终端
pairshell mcp --session <本地引用>       # stdio MCP，与客户授权绑定
pairshell config --server https://your-relay.example
```

MCP 与人工终端共享同一执行占用规则；需要 Agent 独立任务时使用 headless 模式，避免与正在修改系统的交互 Shell 并发。

新短码协议、开源选型和产品流程见 [短码说明](docs/PAIRCODE.md)、[产品简化方案](docs/PRODUCT-SIMPLIFICATION.md)。以下旧邀请/SSH 命令仍兼容，属于高级入口。

自建服务默认保留私有准入；运营者需显式加入 `pairshelld -device-pairing` 才开放匿名设备码信箱。这会允许消耗受配额限制的协调/中继资源，客户端执行仍须客户同意。官方实例同样需要运营者升级并启用；客户端不能悄悄修改服务端准入策略。

开源可自建的临时终端配对工具，按 [技术设计与开发规范](PairShell-技术设计与开发规范.md) 实施。当前为开发版本，**尚未达到文档第 17 节完整发布验收标准**。

已实现 Go 双端配对、认证信令、Pion DataChannel、SSH 密钥固定、任务/文件统一授权审计、本机客户控制、官方 MCP SDK stdio、日志校验回放、签名见证服务和部署配置。具体尚未完成的项目见 [验收台账](docs/IMPLEMENTATION.md)。

## 构建

需要 Go 1.26 或以上。客户端运行时无需 Go、Node 或 Python。

```powershell
go test ./...
go vet ./...
go build -o bin/pairshell-windows-amd64.exe ./cmd/pairshell
go build -o bin/pairshelld-windows-amd64.exe ./cmd/pairshelld
```

本工作目录已下载并校验便携 Go，可用 `.tools/go/bin/go.exe` 替换 `go`；工具链不属于发布内容。

## 使用

先按 [部署说明](docs/deployment.md) 启动带可信 TLS 的协调服务，可选配置 coturn。两端必须信任服务端证书，不提供跳过 TLS 校验选项。

商家：

```powershell
$env:PAIRSHELL_CREATE_TOKEN = '<部署级创建令牌>'
./bin/pairshell-windows-amd64.exe merchant --server https://relay.example --session ./current-session.json
```

将输出的一次性邀请通过已有可信渠道发送给客户。邀请包含秘密，避免保存到日志、工单或公开渠道。商家按终端提示输入客户通过可信渠道读出的本次校验码。

客户：

```powershell
./bin/pairshell-windows-amd64.exe customer --root C:/Work --logs ./data
```

通过标准输入粘贴邀请，打开终端显示的本机控制链接，输入商家从独立可信渠道读出的本次校验码，检查终端、任务、文件和 AI 权限后授权。页面关闭会先暂停再结束。暂停期间已经运行的程序可能继续运行，结束后已经发生的修改需要另行处理。终端拥有当前系统账号的权限。

连接后商家终端会显示本次 SSH 配置及 MCP 命令：

```powershell
ssh -F ./current-session.json.ssh_config pairshell-current
./bin/pairshell-windows-amd64.exe mcp --session ./current-session.json
```

MCP 使用 stdio，共七个工具；客户必须单独批准 AI 和对应任务/文件能力。每个适配器固定一个本地引用。保持 merchant 进程运行，Ctrl+C 结束；正常结束删除临时 SSH 密钥及引用。

也可连接后执行一条明确命令（按客户系统选择语法）：

```powershell
./bin/pairshell-windows-amd64.exe merchant --server https://relay.example --command 'Get-Location'
```

日志：

```powershell
./bin/pairshell-windows-amd64.exe verify ./data/session-<id>
./bin/pairshell-windows-amd64.exe replay ./data/session-<id>
```

回放将控制字符显示为转义文本，避免直接执行日志中的终端控制序列。日志可能包含密码、输出及文件路径，应保护原始目录。见证服务独立提供 API；当前 CLI 尚未接入周期检查点调度，不应将本地日志完整标记解释为已获得远端见证。

## 协议及边界

- [配对、信令、授权协议](docs/protocol.md)
- [审计与见证格式及限制](docs/audit.md)
- [实施计划、发布验收缺口](docs/IMPLEMENTATION.md)
- [部署与运维](docs/deployment.md)

尚需独立安全审查、真实三系统与网络矩阵、任务进程清理加强、发布签名与公证。跨平台构建成功不代表真机兼容性验收通过。

## 0.5 商家与客户端独立窗口

Mac 包含「PairShell 商家端.app」和「PairShell 客户端.app」；Windows 包含 `PairShell.Merchant.exe` 与 `PairShell.Customer.exe`。两端共享 UI，可在商家窗口创建邀请、核对身份、执行独立命令与查看输出。部署入口及私有令牌位置见 [HostHatch 部署](docs/HOSTHATCH-DEPLOYMENT.md)，跨机器验证见 [0.5 验证记录](docs/VERIFICATION-0.5.md)。Linux CLI 已在 HostHatch 实测，不宣称具有 Linux 原生桌面窗口。
