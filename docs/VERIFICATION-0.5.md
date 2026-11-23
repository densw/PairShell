# 0.5 双端与 HostHatch 实测记录

2026-11-16，协同服务 `https://167.104.165.174:9443`。

| 校验 | 结果 |
| --- | --- |
| Windows / Mac / HostHatch Linux 经公网协调服务完成 SSH/RPC 授权、日志测试 | 通过 |
| 商家窗口：创建邀请、比对校验码、确认、执行、暂停后拒绝命令、结束 | Windows、Mac、Linux 测试驱动通过 |
| Windows 商家 → Mac 客户真实跨机器执行 echo | 通过 |
| Mac 商家 → HostHatch Linux 客户真实跨机器执行 echo | 通过 |
| HostHatch Linux 商家 → Windows 客户真实跨机器执行 echo | 通过 |
| coturn relay-only Pion：UDP、TCP、TLS | Windows、Mac、HostHatch Linux 均通过三种传输 |
| Mac 独立商家 App 和 Windows 独立商家 exe | 原生窗口内加载商家 UI、正常退出通过 |
| Go 全量测试、go vet | Windows / Mac 通过 |
| Mac Universal 构建、ad-hoc 签名、DMG 校验 | 通过 |

跨机器探针只在临时目录执行固定 echo，自动确认仅存在于 opt-in 测试，不是生产客户端功能。相关测试入口 `cross_machine_test.go`、`merchant_flow_test.go`、`public_turn_test.go`。未设置测试环境变量时，外部探针不运行。

专项修复：结束时原先先拆除 Pion 通道再关闭虚拟流，客户可能迟迟不能收尾。现在虚拟流先发送关闭并等待短时绑定会话的 close_ack，再拆除底层路径；公网商家流程和跨机器结束均重测通过。

交付：`PairShell-0.5.0-macOS-universal.dmg` 含独立客户与商家两个 App；`PairShell-0.5.0-Windows-x64.zip` 含 `PairShell.Customer.exe`、`PairShell.Merchant.exe` 与后台。Linux 提供 CLI；没有宣称 Linux 桌面 GUI 已实现。商家原生窗口主要提供独立命令和会话控制，SSH/MCP 适配器继续可用；窗口内完整交互终端仍待接入成熟终端组件。

仍未完成：Windows Authenticode / Apple Developer ID、公证；所有真实 NAT/休眠/断网矩阵；IPv6 全路径；Intel/ARM Linux 真机；周期见证及完整发布安全门禁。主机既有 IP 证书为短周期证书，续期同步 hook 已部署，仍需持续监控既有续期计时器成功情况。

部署后的最终公网复测曾出现一次连接重置/配对失败，健康检查与创建接口随后确认正常，再次完整运行公网 RPC 与商家窗口流程通过；未把单次网络失败解释成确定的代码缺陷，也未自动重放操作。活动房间已修正为领取后使用独立两小时期限，相关回归测试在 Windows 和 Mac 通过。
