package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishRoute(t *testing.T) {
	s := setupAdmin(t)
	body := `{"name":"new-route","host":"","paths":[{"path":"/new","type":"prefix"}],"upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["name"] != "new-route" {
		t.Fatalf("expected route name 'new-route', got %s", result["name"])
	}
}

func TestPublishRoute_Duplicate(t *testing.T) {
	s := setupAdmin(t)
	// "api" route already exists from setupAdmin
	body := `{"name":"api","host":"","paths":[{"path":"/dup","type":"prefix"}],"upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishRoute_MissingName(t *testing.T) {
	s := setupAdmin(t)
	body := `{"paths":[{"path":"/new","type":"prefix"}],"upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishRoute_MissingUpstream(t *testing.T) {
	s := setupAdmin(t)
	body := `{"name":"new-route","paths":[{"path":"/new","type":"prefix"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishRoute_MissingPaths(t *testing.T) {
	s := setupAdmin(t)
	body := `{"name":"new-route","upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishRoute_InvalidBody(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRoute(t *testing.T) {
	s := setupAdmin(t)
	body := `{"paths":[{"path":"/updated","type":"exact"}],"upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/routes/api", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["name"] != "api" {
		t.Fatalf("expected route name 'api', got %s", result["name"])
	}
}

func TestUpdateRoute_NotFound(t *testing.T) {
	s := setupAdmin(t)
	body := `{"paths":[{"path":"/updated","type":"exact"}],"upstream":"backend"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/routes/nonexistent", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateRoute_MissingUpstream(t *testing.T) {
	s := setupAdmin(t)
	body := `{"paths":[{"path":"/updated","type":"exact"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/routes/api", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRoute(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/routes/api", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["name"] != "api" {
		t.Fatalf("expected route name 'api', got %s", result["name"])
	}

	// Verify route is gone
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)

	var routes []map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected 0 routes after delete, got %d", len(routes))
	}
}

func TestDeleteRoute_NotFound(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/routes/nonexistent", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishDoc(t *testing.T) {
	s := setupAdmin(t)
	body := `{"route_name":"api","description":"User API documentation","version":"1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["route_name"] != "api" {
		t.Fatalf("expected route_name 'api', got %s", result["route_name"])
	}
}

func TestPublishDoc_MissingRouteName(t *testing.T) {
	s := setupAdmin(t)
	body := `{"description":"Missing route name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishDoc_InvalidBody(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishDoc_Update(t *testing.T) {
	s := setupAdmin(t)

	// Publish initial doc
	body := `{"route_name":"api","description":"Initial description","version":"1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Update doc
	body2 := `{"route_name":"api","description":"Updated description","version":"2.0.0"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body2))
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w2.Code)
	}

	// Verify update
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/docs/api", nil)
	w3 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w3, req3)

	var doc APIDoc
	if err := json.Unmarshal(w3.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Description != "Updated description" {
		t.Fatalf("expected updated description, got %s", doc.Description)
	}
	if doc.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %s", doc.Version)
	}
	if doc.PublishedAt == "" {
		t.Fatal("expected published_at to be set")
	}
	if doc.UpdatedAt == "" {
		t.Fatal("expected updated_at to be set")
	}
}

func TestListDocs(t *testing.T) {
	s := setupAdmin(t)

	// Publish two docs
	body1 := `{"route_name":"api","description":"API docs","version":"1.0.0"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body1))
	w1 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w1, req1)

	body2 := `{"route_name":"web","description":"Web docs","version":"1.0.0"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body2))
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)

	// List docs
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	w3 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}
	var docs []APIDoc
	if err := json.Unmarshal(w3.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
}

func TestListDocs_Empty(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var docs []APIDoc
	if err := json.Unmarshal(w.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected 0 docs, got %d", len(docs))
	}
}

func TestGetDoc(t *testing.T) {
	s := setupAdmin(t)

	// Publish a doc
	body := `{"route_name":"api","description":"API documentation","version":"1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// Get the doc
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/docs/api", nil)
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var doc APIDoc
	if err := json.Unmarshal(w2.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.RouteName != "api" {
		t.Fatalf("expected route_name 'api', got %s", doc.RouteName)
	}
	if doc.Description != "API documentation" {
		t.Fatalf("expected description 'API documentation', got %s", doc.Description)
	}
}

func TestGetDoc_NotFound(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/nonexistent", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDoc(t *testing.T) {
	s := setupAdmin(t)

	// Publish a doc
	body := `{"route_name":"api","description":"API documentation","version":"1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// Delete the doc
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/docs/api", nil)
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify doc is gone
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/docs/api", nil)
	w3 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w3.Code)
	}
}

func TestDeleteDoc_NotFound(t *testing.T) {
	s := setupAdmin(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/docs/nonexistent", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
