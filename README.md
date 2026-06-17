# bolt-proxy

Tunnels Bolt (Neo4j) traffic from Cartography jobs in FSS/dev-gcp to Neo4j in prod-gcp over WebSocket (TLS + Entra ID M2M auth), since direct cluster-to-cluster networking is unavailable.

```
Cartography → bolt://neo4j-bolt.appsec.svc → [bolt-proxy-outbound]
  → WSS + Bearer token → [bolt-proxy-inbound, prod-gcp]
    → bolt://neo4j-bolt.appsec.svc → Neo4j
```

## Binaries

- **`cmd/outbound`** — runs in dev-fss, dev-gcp, prod-fss. Listens on `:7687`, exposed as a `neo4j-bolt` ClusterIP Service so Cartography's `--neo4j-uri` requires no changes.
- **`cmd/inbound`** — runs in prod-gcp behind `appsec-bolt-proxy.intern.nav.no`. Validates Entra ID M2M tokens before forwarding to Neo4j.

## Auth

Outbound fetches tokens via `NAIS_TOKEN_ENDPOINT`. Inbound validates via `NAIS_TOKEN_INTROSPECTION_ENDPOINT`. No JWT libraries — all delegated to the Nais token sidecar.

## Deploy

Merge to `main` builds and deploys all four manifests via GitHub Actions.

## Contact

For any questions, issues, or feature requests, please reach out to the AppSec team:
- Internal: Either our slack channel [#appsec](https://nav-it.slack.com/archives/C06P91VN27M) or contact a [team member](https://teamkatalogen.nav.no/team/02ed767d-ce01-49b5-9350-ee4c984fd78f) directly via slack/teams/mail.
- External: [Open GitHub Issue](https://github.com/navikt/bolt-proxy/issues/new/choose)