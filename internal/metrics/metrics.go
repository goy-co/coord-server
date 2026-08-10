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
			Help: "Total de pedidos HTTP processados por método, caminho e código de estado.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coord_http_request_duration_seconds",
			Help:    "Duração do processamento de pedidos HTTP em segundos.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	NodesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "coord_nodes_total",
			Help: "Número total de nós registados por estado.",
		},
		[]string{"status"},
	)

	RelaysActiveTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "coord_relays_active_total",
			Help: "Número total de relays em estado ativo no registry.",
		},
	)

	VPNKeysGeneratedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_vpn_keys_generated_total",
			Help: "Total de pre-auth keys geradas com sucesso no Headscale.",
		},
	)

	VPNErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_vpn_errors_total",
			Help: "Total de erros de comunicação com a API do Headscale.",
		},
	)

	AuthFailuresTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_auth_failures_total",
			Help: "Total de tentativas de autenticação falhadas na API.",
		},
	)

	RateLimitRejectedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "coord_ratelimit_rejected_total",
			Help: "Total de pedidos HTTP rejeitados por exceder o rate limit.",
		},
	)

	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "coord_db_query_duration_seconds",
			Help:    "Duração das queries e operações na base de dados SQLite.",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		},
		[]string{"operation"},
	)
)

// NormalizePath normaliza o caminho HTTP para evitar a explosão de cardinalidade no Prometheus.
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
