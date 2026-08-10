package middleware

import (
	"encoding/json"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goy-co/coord-server/internal/config"
	"github.com/goy-co/coord-server/internal/metrics"
)

type rateLimitErrorResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

func writeTooManyRequests(w http.ResponseWriter, retryAfterSeconds int) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(rateLimitErrorResponse{
		Error:  "rate limit exceeded",
		Reason: "limite de pedidos HTTP excedido",
	})
}

type ipBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter gere os buckets de tokens para limitação de taxa baseada no IP do cliente.
type IPRateLimiter struct {
	mu            sync.RWMutex
	buckets       map[string]*ipBucket
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// NewIPRateLimiter instancia um novo IPRateLimiter e inicia a goroutine de limpeza em background.
func NewIPRateLimiter() *IPRateLimiter {
	limiter := &IPRateLimiter{
		buckets:       make(map[string]*ipBucket),
		cleanupTicker: time.NewTicker(5 * time.Minute),
		stopChan:      make(chan struct{}),
	}

	go limiter.startCleanupLoop()

	return limiter
}

func (l *IPRateLimiter) Close() {
	l.cleanupTicker.Stop()
	close(l.stopChan)
}

func (l *IPRateLimiter) startCleanupLoop() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.cleanupStaleBuckets()
		case <-l.stopChan:
			return
		}
	}
}

func (l *IPRateLimiter) cleanupStaleBuckets() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, b := range l.buckets {
		if b.lastRefill.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}

// Allow verifica se o IP tem tokens suficientes para efetuar o pedido segundo a taxa e burst.
func (l *IPRateLimiter) Allow(ip string, rpm int, burst int) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	fillRate := float64(rpm) / 60.0 // tokens por segundo

	b, exists := l.buckets[ip]
	if !exists {
		l.buckets[ip] = &ipBucket{
			tokens:     float64(burst) - 1.0,
			lastRefill: now,
		}
		return true, 0
	}

	// Refill de tokens decorrido desde a última chamada
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * fillRate
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	// Calcular tempo necessário para recuperar 1 token inteiro
	missing := 1.0 - b.tokens
	retryAfterSec := int(math.Ceil(missing / fillRate))
	if retryAfterSec <= 0 {
		retryAfterSec = 1
	}

	return false, retryAfterSec
}

// RateLimitMiddleware é o middleware HTTP que impõe os limites de taxa por IP.
func RateLimitMiddleware(limiter *IPRateLimiter, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Isentar caminhos públicos
			for _, pubPath := range cfg.Auth.PublicPaths {
				if path == pubPath || strings.HasPrefix(path, pubPath+"/") {
					next.ServeHTTP(w, r)
					return
				}
			}

			clientIP := ExtractClientIP(r)

			rpm := cfg.RateLimit.RequestsPerMinute
			burst := cfg.RateLimit.Burst

			// Limite mais generoso para pedidos de heartbeat (/relays/{node_id} PUT)
			if r.Method == http.MethodPut && strings.HasPrefix(path, "/relays/") {
				rpm = cfg.RateLimit.HeartbeatRPM
			}

			allowed, retryAfter := limiter.Allow(clientIP, rpm, burst)
			if !allowed {
				slog.Warn("Rate limit excedido para IP do cliente",
					slog.String("ip", clientIP),
					slog.String("path", path),
					slog.Int("retry_after_seconds", retryAfter),
				)
				metrics.RateLimitRejectedTotal.Inc()
				writeTooManyRequests(w, retryAfter)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ExtractClientIP extrai o IP real do cliente (considerando headers X-Forwarded-For).
func ExtractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
