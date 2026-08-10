package metrics_test

import (
	"testing"

	"github.com/goy-co/coord-server/internal/metrics"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/health", "/health"},
		{"/info", "/info"},
		{"/metrics", "/metrics"},
		{"/v1/nodes/register", "/v1/nodes/register"},
		{"/v1/nodes/goy-node-12345", "/v1/nodes/:id"},
		{"/relays", "/relays"},
		{"/relays/goy-node-7890", "/relays/:id"},
		{"/v1/vpn/status", "/v1/vpn/status"},
	}

	for _, tt := range tests {
		got := metrics.NormalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizePath(%q) = %q, esperava %q", tt.input, got, tt.expected)
		}
	}
}

func TestPrometheusMetricsRecordings(t *testing.T) {
	metrics.AuthFailuresTotal.Inc()
	metrics.RateLimitRejectedTotal.Inc()
	metrics.VPNKeysGeneratedTotal.Inc()
	metrics.VPNErrorsTotal.Inc()
	metrics.NodesTotal.WithLabelValues("active").Set(5)
	metrics.RelaysActiveTotal.Set(3)
}
