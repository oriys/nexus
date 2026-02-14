package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nexus/internal/auth"
)

// Auth returns a middleware that enforces authentication.
func Auth(authenticator auth.Authenticator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, err := authenticator.Authenticate(r)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "unauthorized",
					"message": err.Error(),
				})
				return
			}
			ctx := auth.IdentityToContext(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
