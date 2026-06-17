package config

import (
	"fmt"
	"os"
)

// Outbound holds configuration for the outbound proxy binary.
type Outbound struct {
	// BoltListenAddr is the TCP address to accept Bolt connections from Cartography.
	// Default: :7687
	BoltListenAddr string

	// HealthAddr is the HTTP address for /isalive, /isready and /metrics.
	// Default: :8080
	HealthAddr string

	// InboundWSURL is the WebSocket URL of the inbound proxy.
	// Example: wss://appsec-bolt-proxy.intern.nav.no/ws
	InboundWSURL string

	// TokenTarget is the Entra ID audience for M2M token requests.
	// Example: api://prod-gcp.appsec.bolt-proxy-inbound/.default
	TokenTarget string

	// TokenEndpoint is injected by Nais (NAIS_TOKEN_ENDPOINT).
	TokenEndpoint string
}

// Inbound holds configuration for the inbound proxy binary.
type Inbound struct {
	// HealthAddr is the HTTP address for /isalive, /isready, /metrics and /ws.
	// Default: :8080
	HealthAddr string

	// Neo4jBoltAddr is the ClusterIP:port of the Neo4j Bolt service.
	// Default: neo4j-bolt.appsec.svc.cluster.local:7687
	Neo4jBoltAddr string

	// IntrospectionEndpoint is injected by Nais (NAIS_TOKEN_INTROSPECTION_ENDPOINT).
	IntrospectionEndpoint string
}

// LoadOutbound reads outbound proxy configuration from environment variables.
// Returns an error if any required variable is missing.
func LoadOutbound() (Outbound, error) {
	cfg := Outbound{
		BoltListenAddr: envOr("BOLT_LISTEN_ADDR", ":7687"),
		HealthAddr:     envOr("HEALTH_ADDR", ":8080"),
		InboundWSURL:   os.Getenv("INBOUND_WS_URL"),
		TokenTarget:    os.Getenv("TOKEN_TARGET"),
		TokenEndpoint:  os.Getenv("NAIS_TOKEN_ENDPOINT"),
	}

	var missing []string
	if cfg.InboundWSURL == "" {
		missing = append(missing, "INBOUND_WS_URL")
	}
	if cfg.TokenTarget == "" {
		missing = append(missing, "TOKEN_TARGET")
	}
	if cfg.TokenEndpoint == "" {
		missing = append(missing, "NAIS_TOKEN_ENDPOINT")
	}
	if len(missing) > 0 {
		return Outbound{}, fmt.Errorf("missing required environment variables: %v", missing)
	}

	return cfg, nil
}

// LoadInbound reads inbound proxy configuration from environment variables.
// Returns an error if any required variable is missing.
func LoadInbound() (Inbound, error) {
	introspectionEndpoint := os.Getenv("NAIS_TOKEN_INTROSPECTION_ENDPOINT")
	if introspectionEndpoint == "" {
		return Inbound{}, fmt.Errorf("missing required environment variable: NAIS_TOKEN_INTROSPECTION_ENDPOINT")
	}

	return Inbound{
		HealthAddr:            envOr("HEALTH_ADDR", ":8080"),
		Neo4jBoltAddr:         envOr("NEO4J_BOLT_ADDR", "neo4j-bolt.appsec.svc.cluster.local:7687"),
		IntrospectionEndpoint: introspectionEndpoint,
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
