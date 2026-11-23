# Windows 测试与防火墙

本机测试使用固定可执行路径，避免 Go 默认临时 `go-build*/...test.exe` 每次变化触发防火墙提示：

```powershell
./scripts/test-windows.ps1
# 只编译，供首次配置防火墙
./scripts/test-windows.ps1 -CompileOnly
```

测试二进制位于 `.tools/test-bin`；脚本在各包目录下运行，保留 Go 测试所需工作目录。

2026-11-20 已在本机为本次27个实际程序建立54条 TCP/UDP 入站允许规则。规则组为 `PairShell LAN Tests`，限定专用网络与 `192.168.31.0/24`，未关闭防火墙、未添加全局端口放行或 Defender 扫描排除。Windows 原生回环连接不另加远程地址规则。

可用管理员 PowerShell 运行 `scripts/allow-test-firewall.ps1` 重建精确程序规则。脚本按路径固定规则名、记录 SHA-256，并输出 `.tools/firewall-allow-report.json`；仅处理明确列出的项目/安装程序及固定测试目录，不信任整个临时文件夹。移到其他路径的程序需要重新配置。
