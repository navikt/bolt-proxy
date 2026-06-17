# Bolt Tunnel Proxies — Project Init

Two Go binaries that tunnel Cartography/TPT's Bolt traffic to Neo4j in
prod-gcp over the internet, since direct cluster-to-cluster networking isn't
available.

```
Cartography → Bolt/TCP → [outbound-proxy] → WebSocket (TLS + Entra token) →
  internet → [inbound-proxy, prod-gcp] → Bolt/TCP → Neo4j (ClusterIP)
```

Cartography's driver config is unchanged (`bolt://localhost:7687`).

**Transport: WebSocket.** HTTP CONNECT was tested against NAIS ingress and
rejected (405). WS upgrade was tested and succeeds end-to-end.

**WS library: `github.com/coder/websocket`.** Do not use
`golang.org/x/net/websocket` — retired upstream, no ping/continuation-frame
support, breaks on long-lived connections.

---

## Repo layout

```
bolt-tunnel/
├── cmd/
│   ├── outbound/main.go
│   └── inbound/main.go
├── internal/
│   ├── bridge/bridge.go     # bidirectional byte pump, shared by both
│   ├── auth/
│   │   ├── token.go         # outbound: fetch token via NAIS_TOKEN_ENDPOINT
│   │   └── validate.go      # inbound: JWKS fetch/cache + claim checks
│   └── config/config.go
├── deploy/
│   ├── outbound-app.yaml
│   └── inbound-app.yaml
├── Dockerfile.outbound
└── Dockerfile.inbound
```

## Dependencies

- `github.com/coder/websocket`
- One JWT library for inbound validation — pick one, don't add a second.
- Everything else: stdlib.

## Outbound proxy

- Listens locally for Cartography's Bolt TCP connections.
- Per connection: fetch Entra token (NAIS M2M flow), open WSS to inbound
  proxy with `Authorization: Bearer <token>`, bridge bytes.
- On auth rejection, retry once with forced token refresh, then fail.

## Inbound proxy

- Accepts WS upgrade requests on the public endpoint.
- Validate token before upgrading: signature (JWKS), `aud`, `iss`, `exp`,
  caller app identity against an explicit allowlist. Reject before upgrade.
- On success: dial Neo4j ClusterIP Bolt port, bridge bytes.
- Log connection metadata (identity, accept/reject, duration, bytes) on
  every attempt.

## Shared bridge

One function, both binaries, two goroutines pumping bytes each direction
until either side closes. Test with `net.Pipe()`.

## Decide before coding

- Which JWT claim carries caller app identity (`azp` vs `appid`) — check a
  real token.
- Outbound topology: sidecar per Cartography pod vs standalone Service.
- Idle/max connection lifetime for tunnels.
- Concurrent connection cap per pod.
- Separate health-check path, not the WS path.

## Test order

1. `bridge` unit tests via `net.Pipe()`.
2. `validate` unit tests against locally generated test JWKS.
3. Integration: outbound → inbound → throwaway TCP echo server, localhost.
4. Full chain against real Neo4j + real Cartography sync.
