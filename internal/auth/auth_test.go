package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newAuthenticator() *APIKeyAuthenticator {
	return NewAPIKeyAuthenticator(map[string]string{
		"key-abc": "alice",
		"key-xyz": "bob",
	})
}

func TestAPIKeyAuth_ValidHeaderKey(t *testing.T) {
	a := newAuthenticator()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "key-abc")

	id, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Subject != "alice" {
		t.Fatalf("expected subject alice, got %s", id.Subject)
	}
	if id.Source != "apikey" {
		t.Fatalf("expected source apikey, got %s", id.Source)
	}
}

func TestAPIKeyAuth_ValidQueryKey(t *testing.T) {
	a := newAuthenticator()
	r := httptest.NewRequest(http.MethodGet, "/?api_key=key-xyz", nil)

	id, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Subject != "bob" {
		t.Fatalf("expected subject bob, got %s", id.Subject)
	}
	if id.Source != "apikey" {
		t.Fatalf("expected source apikey, got %s", id.Source)
	}
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	a := newAuthenticator()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := a.Authenticate(r)
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	a := newAuthenticator()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-API-Key", "wrong-key")

	_, err := a.Authenticate(r)
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestAPIKeyAuth_HeaderOverQuery(t *testing.T) {
	a := newAuthenticator()
	r := httptest.NewRequest(http.MethodGet, "/?api_key=key-xyz", nil)
	r.Header.Set("X-API-Key", "key-abc")

	id, err := a.Authenticate(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Subject != "alice" {
		t.Fatalf("expected subject alice (from header), got %s", id.Subject)
	}
}

func TestIdentityContext(t *testing.T) {
	id := &Identity{Subject: "alice", Claims: map[string]any{"role": "admin"}, Source: "apikey"}
	ctx := IdentityToContext(context.Background(), id)
	got := GetIdentity(ctx)
	if got == nil {
		t.Fatal("expected identity, got nil")
	}
	if got.Subject != "alice" {
		t.Fatalf("expected subject alice, got %s", got.Subject)
	}
	if got.Claims["role"] != "admin" {
		t.Fatalf("expected claim role=admin, got %v", got.Claims["role"])
	}
}

func TestGetIdentity_NoIdentity(t *testing.T) {
	got := GetIdentity(context.Background())
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
