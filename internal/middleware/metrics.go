package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/goy-co/coord-server/internal/metrics"
)

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// MetricsMiddleware records latency metrics and HTTP request counts for Prometheus.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		srw := &statusResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(srw, r)

		duration := time.Since(start).Seconds()
		normPath := metrics.NormalizePath(r.URL.Path)
		statusStr := strconv.Itoa(srw.statusCode)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, normPath, statusStr).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, normPath).Observe(duration)
	})
}
