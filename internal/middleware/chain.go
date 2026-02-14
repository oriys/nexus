package middleware

import (
	"log/slog"
	"net/http"
)

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares to the handler in reverse order so that the
// first middleware in the list is the outermost (executed first).
// Each middleware is wrapped with panic recovery to prevent cascade failures.
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		next := middlewares[i](handler)
		handler = recoverWrap(next)
	}
	return handler
}

// recoverWrap wraps a handler with panic recovery.
func recoverWrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("middleware panic recovered",
					slog.Any("error", err),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}
