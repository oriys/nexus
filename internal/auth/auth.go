package auth

import (
	"context"
	"errors"
	"net/http"
)

type contextKey string

const identityKey contextKey = "auth.identity"

// Identity represents an authenticated identity.
type Identity struct {
	Subject string
	Claims  map[string]any
	Source  string // authentication source, e.g. "apikey"
}

// Authenticator is the interface for authentication strategies.
type Authenticator interface {
	Authenticate(r *http.Request) (*Identity, error)
}

// APIKeyAuthenticator validates requests using API keys.
type APIKeyAuthenticator struct {
	keys map[string]string // key -> subject mapping
}

var (
	ErrMissingAPIKey = errors.New("missing API key")
	ErrInvalidAPIKey = errors.New("invalid API key")
)

// NewAPIKeyAuthenticator creates an authenticator with a key→name mapping.
func NewAPIKeyAuthenticator(keys map[string]string) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{keys: keys}
}

// Authenticate validates the API key from the X-API-Key header or api_key query parameter.
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.URL.Query().Get("api_key")
	}
	if key == "" {
		return nil, ErrMissingAPIKey
	}

	name, ok := a.keys[key]
	if !ok {
		return nil, ErrInvalidAPIKey
	}

	return &Identity{
		Subject: name,
		Claims:  map[string]any{},
		Source:  "apikey",
	}, nil
}

// GetIdentity extracts the identity from the context.
func GetIdentity(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}

// IdentityToContext stores the identity in the context.
func IdentityToContext(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}
