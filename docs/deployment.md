# Self hosting

Build `go build -o bin/pairshelld ./cmd/pairshelld`. Set distinct high-entropy PAIRSHELL_CREATE_TOKEN, PAIRSHELL_TURN_SECRET and PAIRSHELL_WITNESS_TOKEN environment values; avoid shell history and process arguments for secrets. Set PAIRSHELL_TURN_URLS to comma-separated URLs, e.g. turn:relay.example:3478?transport=udp,turns:relay.example:5349?transport=tcp.

`pairshelld --listen 0.0.0.0:8443 --tls-cert /certs/fullchain.pem --tls-key /certs/privkey.pem --witness-dir /data/receipts --witness-key /data/witness.key`

TLS is mandatory for a non-loopback listener. Run under a dedicated unprivileged OS user, restrict key and data directory access, use a systemd service or Windows service supervisor, restart on failure. Room state is in memory and intentionally lost on restart; witness key and receipt directory must persist. Back up the witness key securely and pin its public key externally. Never print invitations or terminal contents in deployment logs.

For local development, default 127.0.0.1:8080 permits HTTP. This is not a public deployment setting. The creation token is merchant-only. Witness API currently requires a distinct instance token in addition to both-end declaration signatures; distribute it out of band to authorized clients, not as a room creation token. Automatic witness token distribution is not implemented.

Witness POST /witness/register accepts audit.Declaration; POST /witness/checkpoint accepts audit.Checkpoint and returns audit.Receipt. Both require Authorization: Bearer <witness token>, bounded strict JSON and valid signatures. Registration is mandatory before the first checkpoint. Do not expose the witness API without authentication and external rate/disk quota controls.

For Docker on Linux, copy [the public TURN template](../deploy/turnserver.conf.template) to a file outside the checkout. Set its realm, external address and shared secret. Store the environment file and TLS material outside the checkout, restrict their permissions, and grant the container users read access to the files they need. Set `PAIRSHELL_CERT_DIR` to the certificate directory and `PAIRSHELL_TURN_CONFIG` to the rendered TURN file, both as absolute paths. Pass the private environment file with `docker compose --env-file /path/to/private.env -f deploy/compose.yaml`. Validate with `config --quiet` so the resolved credentials are not printed, then start with `up --build -d`.

The coordinator publishes TCP 443. The coturn image is pinned by release and registry index digest. The sample keeps TURN/UDP and TURN/TCP on 3478 and TURN/TLS over TCP on 5349, with UDP relay ports 49160–49200. DTLS and RFC 6062 TCP relay endpoints are disabled. Configure the public/private NAT mapping explicitly. The supplied coturn limits are 1 MiB/s per allocation in each direction, 32 MiB/s of server allocation capacity, eight allocations per username and 40 total. They apply to traffic handled by TURN. Apply firewall or provider-level packet rate limits for bursts that can fill the upstream network link before reaching coturn.

For a private LAN installation, restrict ingress to the LAN or VPN. Set `PAIRSHELL_LAN_IP` in the private environment file and render [the LAN template](../deploy/lan/turnserver.conf.template) with that address and the intended IPv4 relay-peer range. Include the relay and expected LAN peers in the allowed range. Keep metadata, management and other sensitive subnets unreachable from the relay. Never publish the LAN profile directly to the internet.

The official public endpoint is information clients need to connect. Publish a hostname with a valid certificate. DNS can hide an IP from the README, but anyone connecting directly can still learn it. To restrict access to an origin, place the coordinator behind a proxy or VPN and firewall the origin so only that ingress can reach it. TURN needs a reachable relay endpoint with UDP support. Use a separate TURN host or a compatible network ingress if the coordinator origin must remain private.

The coordinator uses the socket peer address for limits by default. For a reverse proxy deployment, pass `--trusted-proxies` with only the trusted proxy IPs or CIDRs and have the edge proxy set or correctly append `X-Forwarded-For`. Keep the origin inaccessible to clients outside that proxy path. Clients behind the same NAT share the per-source limits.

Customer sessions end after 10 minutes without an authorized operation. To choose a different duration, set `PAIRSHELL_IDLE_TIMEOUT` on the customer side before starting the app or CLI. Use a positive Go duration such as `15m`. Heartbeats, lease renewals and passive viewing do not reset the timer. The two-hour hard room limit still applies.

Set host disk quotas for witness data and restrict relay egress to exclude provider-specific infrastructure and metadata. Verify firewall rules, IPv6 policy and real NAT paths in staging before updating production.
