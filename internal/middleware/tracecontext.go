package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

const traceIDKey contextKey = "trace_id"

// GetTraceID returns the trace ID from the context.
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return ""
}

// TraceContext returns a middleware that ensures W3C Trace Context headers exist.
func TraceContext() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceparent := r.Header.Get("traceparent")
			if traceparent == "" {
				traceID := generateTraceID()
				spanID := generateSpanID()
				traceparent = fmt.Sprintf("00-%s-%s-01", traceID, spanID)
				r.Header.Set("traceparent", traceparent)
			}
			traceID := extractTraceID(traceparent)
			ctx := context.WithValue(r.Context(), traceIDKey, traceID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateTraceID generates a 16-byte random trace ID as 32 hex chars.
func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(b)
}

// generateSpanID generates an 8-byte random span ID as 16 hex chars.
func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", 16)
	}
	return hex.EncodeToString(b)
}

// extractTraceID extracts the trace ID from a traceparent header value.
// traceparent format: "version-traceID-spanID-flags"
func extractTraceID(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
