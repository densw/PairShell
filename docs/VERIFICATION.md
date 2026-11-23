# 本地验证记录

日期：2026-11-16。环境：Windows x64，便携 Go 1.27.4；源码最低 Go 版本见 go.mod。

- `go test ./... -count=1 -timeout=60s`：全部包通过。包括真实 TLS 协调/Pion/SSH/RPC、任务暂停撤销与审计导出、双 Pion 主路关闭后的自动迁移、原生 SSH 嵌套代理及递归拒绝。
- `go vet ./...`：通过。
- `govulncheck ./...`（v1.9.0）：No vulnerabilities found。此结果为扫描时数据库结果，不代替安全设计审查。
- 邀请解析 fuzz 3 秒：856446 次执行，通过；帧解析 fuzz 3 秒：301348 次执行，通过。
- `scripts/build.ps1`：Windows amd64、Linux amd64/arm64、macOS amd64/arm64 的客户端和服务端交叉编译通过；二进制在 bin，SHA256SUMS 同目录生成。
- 依赖固定在 go.mod/go.sum，版本清单在 DEPENDENCIES.txt，模块许可文本在 THIRD_PARTY_NOTICES.md。

未执行：本机竞态检测（CGO_ENABLED=0，缺少 C 编译器）；已配置 Linux/macOS CI 竞态检测但尚未运行远端 CI。Windows ConPTY 暂停的交互回归在无头环境跳过，PTY 基础输出及 resize 测试通过。未进行 Docker 部署、真实公网 TURN/NAT、macOS/Linux 真机运行、签名公证和独立安全评审。

构建与组件测试结果仅证明当前记录的行为，完整设计的工程缺口见 IMPLEMENTATION.md。
