package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger é um middleware HTTP que regista detalhes estruturados de cada pedido via log/slog.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			duration := time.Since(start)
			reqID := middleware.GetReqID(r.Context())

			slog.Info("HTTP Request",
				slog.String("req_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", duration),
				slog.String("remote_addr", r.RemoteAddr),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
