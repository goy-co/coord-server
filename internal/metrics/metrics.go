package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "coord_http_requests_total",
			Help: "Total HTTP requests processed by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coord_http_request_duration_seconds",
			Help:    "HTTP request processing duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	NodesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "coord_nodes_total",
			Help: "Total number of registered nodes by status.",
		},
		[]string{"status"},
	)

	RelaysActiveTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "coord_relays_active_total",
			Help: "Total number of active relays in the registry.",
		},
	)

	VPNKeysGeneratedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_vpn_keys_generated_total",
			Help: "Total pre-auth keys successfully generated in Headscale.",
		},
	)

	VPNErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_vpn_errors_total",
			Help: "Total API communication errors with Headscale.",
		},
	)

	AuthFailuresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_auth_failures_total",
			Help: "Total failed API authentication attempts.",
		},
	)

	RateLimitRejectedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_ratelimit_rejected_total",
			Help: "Total HTTP requests rejected for exceeding the rate limit.",
		},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coord_db_query_duration_seconds",
			Help:    "Duration of SQLite database queries and operations in seconds.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		},
		[]string{"operation"},
	)
)

// NormalizePath normalizes HTTP paths to prevent cardinality explosion in Prometheus.
func NormalizePath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}

	parts := strings.Split(path, "/")
	normalized := make([]string, 0, len(parts))

	for i, part := range parts {
		if part == "" {
			if i == 0 {
				normalized = append(normalized, "")
			}
			continue
		}

		if i > 0 {
			prev := parts[i-1]
			if (prev == "nodes" || prev == "relays") && part != "register" && part != "status" {
				normalized = append(normalized, ":id")
				continue
			}
		}

		normalized = append(normalized, part)
	}

	res := strings.Join(normalized, "/")
	if res == "" {
		return "/"
	}
	return res
}
