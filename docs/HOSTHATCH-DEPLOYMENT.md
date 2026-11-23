# HostHatch 部署与商家双端

服务入口由实例管理员提供，本地部署记录保留实际地址。

邀请领取期限默认 10 分钟，成功领取后活动房间改用独立的 2 小时上限；控制租约仍为 30 秒，不能靠领取延长已过期邀请。

SSH 管理地址保存在管理员的本机配置。

- 程序 `/opt/pairshell/pairshelld`，systemd `pairshelld.service`，低权限 `pairshell` 用户。
- 配置与凭据 `/etc/pairshell/environment`（root-only）。房间只存内存，见证持久化于 `/var/lib/pairshell`。
- coturn：`pairshell-turn.service`，UDP/TCP 3478、TLS 5349，relay UDP 49160–49200；临时凭据、用户/总配额及内网目标拒绝策略开启。
- HTTPS 使用受信任的证书副本。证书部署钩子同步文件并重启 PairShell 和 TURN。续期重启会中断临时会话。
- systemd 安装脚本从 root 私有目录 `/root/pairshell-stage` 读取 `pairshelld` 和 `turnserver.conf.template`。协调服务安装参数依次为可信 SHA-256、证书目录和 TURN URL 列表。TURN 安装需要已安装的 coturn 4.19.2 或更高版本，参数为中继域名和公网 IPv4 地址。通用部署见 [部署说明](deployment.md)。

商家窗口：HTTPS 地址填上述入口，创建令牌由管理员通过私有渠道提供。服务器管理员可从 root-only 配置恢复。不要把见证令牌或 TURN 共享秘密发给客户。

0.5 双端安装包包含独立商家与客户入口，使用同一套 UI。商家可创建邀请、核对身份、等待客户授权、执行独立命令、查看结果、结束；连接后导出 SSH/MCP 本地会话引用。客户最终决定各项操作是否允许。商家窗口尚未嵌入完整交互终端模拟器；交互终端可通过导出的 SSH 配置使用。

连接校验使用只在临时测试目录执行 echo 的自动化用例，测试内自动同意仅存在于测试代码；生产窗口仍要求人工核对与客户授权。公网 HTTP 健康检查、SSH/RPC 授权检查、商家窗口执行/暂停/结束和 TURN UDP/TCP/TLS relay-only 字节流按实际结果验收，不把监听成功当作中继通过。

安装包仍是开发构建，未做 Apple 公证、Developer ID 或 Windows Authenticode，完整网络故障/进程树清理/独立安全评审缺口仍见 IMPLEMENTATION.md。
