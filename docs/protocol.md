# PairShell v1 协议实现约定

本实现处于开发阶段，尚未经过独立密码协议安全审查，不应直接公开部署用于真实维修。

## 邀请与身份

邀请只通过 HTTPS URL fragment 传递。`sid` 为 16 随机字节的无填充 Base64URL；join、secret 各 32 字节，公钥 32 字节。每次会话重新生成 Ed25519 密钥。邀请解析限制 4096 字节，拒绝重复字段、未知版本、查询串及过期邀请。URL 中过期时间仅作早期拒绝，须在认证转录中重新验证。

创建房间返回的 join_token 必须替换 Invite.Join；服务器不接收 secret。邀请中 MerchantKey 必须与认证转录的 MerchantKey 做恒定时间比较；SessionID、Expires、能力同样必须与邀请/用户确认内容一致。不得仅调用 Verify 后就认为完成身份绑定。

## 认证转录

Go 固定字段结构 Transcript 按声明顺序 encoding/json 编码；字节字段按 JSON Base64 标准编码。字段顺序为 version、session_id、merchant_key、customer_key、merchant_challenge、customer_challenge、expires、capabilities。整数不允许浮点。挑战必须由双方各自独立使用 crypto/rand 生成 32 字节并绑定一次握手；调用方不得使用固定或重复挑战。

认证输入为 UTF-8 `PairShell:handshake:v1` + NUL + role + NUL + 上述精确 JSON 字节。role 只能是 merchant/customer。对输入计算 HMAC-SHA256(secret)，同时使用该角色临时 Ed25519 私钥签名。Verify 验证两者、字段长度、版本、角色与过期时间。

会话校验码为 HMAC-SHA256 的前 12 字节，用十六进制显示。HMAC 密钥是随机生成且仅两端持有的 32 字节邀请 secret，消息为 `PairShell:verification:v2` + NUL + 转录 JSON。协调端可以看到信令里的握手字段。仅有这些字段无法计算新版校验码。双方发送并检查经信令认证的 `verification_version = 2`，缺少该字段或版本不符时结束握手。两端需要同步升级，协调服务可以继续转发原有信令格式。

桌面和 CLI 的每次新会话都要求双方各自输入通过独立可信渠道获得的对方校验码。校验码只在本机与当前握手比较。执行通道要等商家端确认和客户本机核对与授权都完成。

## 信令

Signal 固定结构顺序：version、session_id、role、authorization_epoch、sequence、type、payload。签名输入为 `PairShell:signal:v1` + NUL + JSON，HMAC-SHA256(secret)。payload 上限 256 KiB（HTTP 整体上限还包含编码开销）。每个方向独立从 1 递增，接收器 SignalAuthenticator 固定绑定 SessionID、PeerRole 和 Epoch，锁内验证 MAC 并只接受严格递增序号。SDP、DTLS 指纹和 ICE 候选必须放在已认证 payload 内，先验 MAC 再解释和应用。

## 协调端

所有 /v1 接口使用 POST 与有界单个 JSON 对象。公网监听必须使用 TLS。创建需部署令牌，公开创建必须显式配置。房间仅存于内存，领取一次生效。房间能力令牌在服务内仅保存 SHA256 摘要。默认每个来源按 IPv4 地址或 IPv6 /64 限制为每秒 12 个请求、突发 32 个和并发 8 个。协调服务全局每秒 256 个请求、突发 512 个和并发 64 个。请求体按来源限制为 256 KiB/s、突发 512 KiB，按全局限制为 4 MiB/s、突发 8 MiB。信令每方向最多 64 条和 224 KiB，发送预算每秒补充 128 KiB。后端和代理的部署边界见部署文档。

领取后 join 能力作为客户 API 能力。两端都应将能力令牌视作秘密。公用房间未领取时最多存活 45 秒，领取后两方分别维持最长 30 秒的控制租约。任一方租约过期即回收房间。正常断连也会尝试主动释放房间。

双方客户端完成身份核验且客户同意后，交换认证的就绪信令。协调端收到两个角色各自的就绪请求才发放 TURN 凭据。TURN 使用 coturn REST HMAC-SHA1 格式，用户名为 `expiry:session_id`，凭据为标准 Base64。已签发的凭据由 coturn 按有效期和分配生命周期管理。TURN 目标限制和带宽配额位于 coturn 配置。协调端仍能阻断连接和观察元数据。两端本机核对与授权独立于服务端就绪标记。[三方安全协助](trust.md)

## 客户授权

Gate 串行化授权状态与操作准入。文件工具只读写普通文件，缓慢 I/O 在授权锁之外执行，返回内容或提交改动前复核授权。暂停跨 Reconnecting/Connected 保留，End 增加 epoch、清空能力并取消正在准备的文件操作。连续 10 分钟无有效操作会结束会话，查看记录、心跳和续租不会延长该时间。审计失败会撤销继续执行的权限。

## 待完成的安全门禁

独立审查角色反射、邀请到 SSH 主机密钥的完整绑定、挑战新鲜性与 transcript 编码互操作；为其他语言提供固定编码向量；真实部署源地址限流、TLS、TURN 网络矩阵验收。辅助函数本身不能代替集成调用方对完整握手状态机的验证。
