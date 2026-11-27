# Transport, SSH, and terminal implementation

## Tailcat backend

Both endpoints can opt in with `PAIRSHELL_TRANSPORT=tailcat` before starting the CLI or desktop client. The authenticated hello negotiates `tailcat-v1`; if either endpoint is unset, explicitly uses `webrtc`, or is an older client, both use the existing WebRTC backend. Invalid local transport values fail the handshake. The default remains WebRTC while cross-network acceptance is pending. A negotiated tailcat setup failure returns an error; it does not silently change protocols mid-handshake.

This backend pins `github.com/tailscale/tailcat` v0.8.0 (commit `b9491c13c63925af4119aca70bd21b1691ff3d5b`) under the BSD-3-Clause license and requires Go 1.27.4. The direct `tailscale.com` dependency provides ephemeral node keys and test DERP/STUN support and is also BSD-3-Clause; upstream dependency licenses continue to apply. It uses tailcat as a Go library, with no external CLI, daemon, privileged TUN interface, or Tailscale login. The customer starts an ephemeral WireGuard server accepting only the merchant’s fresh node key, and listens on one dynamically chosen TCP port. Startup, encrypted signaling and dial/accept share a 30-second deadline after consent. The existing mutually pinned SSH protocol runs over that stream; permissions, consent, revocation and auditing stay in the existing SSH/session handlers. Context cancellation closes the stream and userspace backend.

The WireGuard address contains a pre-shared key. Address and client-key descriptors are AES-256-GCM encrypted using a domain-separated key from the invitation secret, fresh nonces and session-ID associated data, inside the existing role/sequence/epoch authenticated signals. Neither descriptor is logged or passed to a subprocess.

Tailcat bootstraps over DERP and magicsock exchanges endpoints and attempts UDP hole punching. The merchant additionally sends bounded disco probes every two seconds to keep discovery active. Route status reports `direct` only for a direct probe endpoint, and `relay` for a DERP result; failed probes report `connecting` instead of stale RTT. It does not compare or switch independent application streams: WireGuard/magicsock handles path changes beneath one TCP stream. No application command replay is introduced.

Set `PAIRSHELL_DERP_MAP_URL` on both endpoints to an operator-controlled DERP map URL. This is a local trusted configuration, not a URL accepted from the coordinator. Production deployments should use HTTPS and a suitable self-hosted DERP service. When unset, tailcat uses its upstream default map and relay terms: see [tailcat documentation](https://github.com/tailscale/tailcat#readme). TURN credentials are used only by the WebRTC backend.

`TestPairTailcatSSH` runs a local DERP/STUN service, completes real pairing/consent and pinned SSH execution, waits for direct UDP discovery, shuts down the bootstrap relay, then executes another SSH command. Descriptor tests reject wrong secrets, session IDs, truncation and tampering. `TestPairTailcatRelay` disables UDP in an isolated process and checks SSH execution and relay status through the local DERP service. Setup and active-session cancellation are tested separately. This proves loopback integration, not improved direct-connect rates across public NATs: CGNAT, symmetric NAT, real firewall/proxy DERP fallback, network changes, long-session recovery and platform runtime matrices still need deployed acceptance.

## WebRTC backend

`internal/remote.Create` creates the room and returns a fragment invitation. Merchant `Connect` requires an explicit transcript verification callback. `ServeCustomer` verifies the invitation/identity proofs before invoking the local consent callback; the callback must return an ACTIVE Gate bound to the same session. Execution handlers must still enforce authorization at each entry/input and audit before execution.

The coordinator receives capability tokens only; the end-to-end secret remains in the client fragment and authenticated signal envelopes. No invitation is placed in subprocess arguments by this package. SSH authenticates the ephemeral merchant key and pins the customer key from the transcript. Forwarding, environment requests, and unknown subsystems/channels are rejected. The exact `pairshell-rpc` channel requires a handler implementing RPC; `pairshell-proxy` permits one nested SSH connection with the same identity keys. Channels are bounded to 16 per SSH connection.

Reliable stream frames use a bounded 16KiB payload and 1MiB receive/unacknowledged budgets. There is one sequence writer per direction; reordered/duplicate frames never enter SSH twice. ACKs return over the ingress path, so the reverse sender can migrate independently. Path replacement retransmits only pending byte frames. Failed sends wait up to 30 seconds (or an earlier caller deadline) and resend the same sequence rather than replaying a command.

Direct-only (host/STUN) and relay-only candidates are gathered concurrently when TURN credentials are available. Both deliver into the same virtual stream; the first working path starts the session without preferring either transport. Each is probed every 2 seconds and compared by the recent 10-sample RTT median. Any positive RTT lower than the current path can win, including 3ms versus 9ms; there is no 10ms or percentage improvement floor. Three consecutive lower-latency observations and a 15-second switch cooldown prevent oscillation. Three probe failures allow failover; an additional 30-second unavailable interval cancels the session. Initial ICE gathering is bounded separately from the customer consent signaling budget, and initial candidate connection waits at most 30 seconds after answer exchange.

## Evidence

Tests exercise real Pion loopback, mutual pairing through a TLS httptest coordinator, pinned SSH exec, forwarding denial, RPC, nested native SSH, duplicate/ordered delivery, ACK rejection, and shared-stream automatic failover after primary PeerConnections close. `TestPionSharedStreamMigration` verifies exactly `beforeafter` without repeated bytes while the write waits for detection and failover. PTY test exercises Windows ConPTY resize, output and process wait.

## Open acceptance work

- The deployed LAN coturn has passed actual relay-only UDP and TCP byte-transfer probes. Cross-public-network NAT/CGNAT matrices, TURN/TLS, credential refresh for long sessions, and expanded platform interactive/cleanup acceptance remain unverified.
- Path comparison uses timeout counters and median latency; richer jitter/loss preference remains future acceptance work. The merchant UI displays the selected local sending path and measured RTT.
- ConPTY's existing wrapper starts the process before the caller assigns its Job Object. A child can escape that startup window. A suspended-create/atomic Job assignment implementation or reviewed wrapper is required before claiming strong process-tree cleanup.
- Unix process groups and Windows Job Objects do not reclaim separately installed system services or scheduled tasks.
- Native nested SSH channel deadlines explicitly return ErrUnsupported rather than pretending that per-channel deadlines are enforced. The underlying migrated stream implements deadlines.
- Race detector requires CGO/toolchain support unavailable in the current environment.

## Exact v1 frame encoding

Each DataChannel binary message is a single UTF-8 JSON object in Go struct order:
`stream_id` (uint32, must be 1), `version` (integer, 1), `session_id` (string), `authorization_epoch` (uint64), `seq` (uint64), `ack` (uint64), `type` (string), optional `payload` (standard Base64 string), `payload_length` (integer decoded bytes).

Types: `data`, `ack`, `close`, and per-path `ping`/`pong`. Data seq starts at 1 independently per direction. ACK is the highest contiguous received data sequence; a future ACK is rejected. Control frames have an empty payload and length 0. Ping/pong uses seq as an opaque probe nonce and is handled before stream delivery; it does not advance data sequence. All frame variants bind version/session/authorization epoch. Payload length must match decoded bytes and cannot exceed 16384; the encoded message must not exceed 32768 bytes. Every frame uses stream 1; multiple SSH channels are multiplexed by SSH inside this stream.

This JSON representation is a local v1 implementation decision, not a claim of interoperability with an external PairShell protocol. A protocol freeze and independent migration/security review remain release gates.
