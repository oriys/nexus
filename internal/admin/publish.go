package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/oriys/nexus/internal/config"
)

// APIDoc represents documentation for a published API route.
type APIDoc struct {
	RouteName   string `json:"route_name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Deprecated  bool   `json:"deprecated"`
	PublishedAt string `json:"published_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DocStore manages API documentation in memory.
type DocStore struct {
	mu   sync.RWMutex
	docs map[string]*APIDoc // route_name → doc
}

// NewDocStore creates a new documentation store.
func NewDocStore() *DocStore {
	return &DocStore{
		docs: make(map[string]*APIDoc),
	}
}

// Get returns the documentation for a route.
func (ds *DocStore) Get(routeName string) (*APIDoc, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	doc, ok := ds.docs[routeName]
	return doc, ok
}

// Set stores documentation for a route.
func (ds *DocStore) Set(doc *APIDoc) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.docs[doc.RouteName] = doc
}

// Delete removes documentation for a route.
func (ds *DocStore) Delete(routeName string) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if _, ok := ds.docs[routeName]; !ok {
		return false
	}
	delete(ds.docs, routeName)
	return true
}

// List returns all stored documentation.
func (ds *DocStore) List() []*APIDoc {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	result := make([]*APIDoc, 0, len(ds.docs))
	for _, doc := range ds.docs {
		result = append(result, doc)
	}
	return result
}

// publishRoute handles POST /api/v1/routes to publish a new route.
func (s *Server) publishRoute(w http.ResponseWriter, r *http.Request) {
	var route config.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if route.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route name is required"})
		return
	}
	if route.Upstream == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route upstream is required"})
		return
	}
	if len(route.Paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route must have at least one path rule"})
		return
	}

	cfg := s.configLoader.Current()
	if cfg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no configuration loaded"})
		return
	}

	// Check for duplicate route name
	for _, existing := range cfg.Routes {
		if existing.Name == route.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "route with name '" + route.Name + "' already exists"})
			return
		}
	}

	// Add route and reload
	newRoutes := make([]config.Route, len(cfg.Routes)+1)
	copy(newRoutes, cfg.Routes)
	newRoutes[len(cfg.Routes)] = route
	cfg.Routes = newRoutes
	s.router.Reload(cfg.Routes)

	writeJSON(w, http.StatusCreated, map[string]string{"message": "route published successfully", "name": route.Name})
}

// updateRoute handles PUT /api/v1/routes/{name} to update an existing route.
func (s *Server) updateRoute(w http.ResponseWriter, r *http.Request) {
	routeName := r.PathValue("name")
	if routeName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route name is required"})
		return
	}

	var route config.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}
	route.Name = routeName

	if route.Upstream == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route upstream is required"})
		return
	}
	if len(route.Paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route must have at least one path rule"})
		return
	}

	cfg := s.configLoader.Current()
	if cfg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no configuration loaded"})
		return
	}

	found := false
	for i, existing := range cfg.Routes {
		if existing.Name == routeName {
			cfg.Routes[i] = route
			found = true
			break
		}
	}

	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route '" + routeName + "' not found"})
		return
	}

	s.router.Reload(cfg.Routes)
	writeJSON(w, http.StatusOK, map[string]string{"message": "route updated successfully", "name": routeName})
}

// deleteRoute handles DELETE /api/v1/routes/{name} to unpublish a route.
func (s *Server) deleteRoute(w http.ResponseWriter, r *http.Request) {
	routeName := r.PathValue("name")
	if routeName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route name is required"})
		return
	}

	cfg := s.configLoader.Current()
	if cfg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no configuration loaded"})
		return
	}

	found := false
	newRoutes := make([]config.Route, 0, len(cfg.Routes))
	for _, existing := range cfg.Routes {
		if existing.Name == routeName {
			found = true
			continue
		}
		newRoutes = append(newRoutes, existing)
	}

	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route '" + routeName + "' not found"})
		return
	}

	cfg.Routes = newRoutes
	s.router.Reload(cfg.Routes)
	writeJSON(w, http.StatusOK, map[string]string{"message": "route unpublished successfully", "name": routeName})
}

// publishDoc handles POST /api/v1/docs to publish API documentation.
func (s *Server) publishDoc(w http.ResponseWriter, r *http.Request) {
	var doc APIDoc
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if doc.RouteName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route_name is required"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if existing, ok := s.docStore.Get(doc.RouteName); ok {
		doc.PublishedAt = existing.PublishedAt
		doc.UpdatedAt = now
	} else {
		doc.PublishedAt = now
		doc.UpdatedAt = now
	}

	s.docStore.Set(&doc)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "documentation published successfully", "route_name": doc.RouteName})
}

// listDocs handles GET /api/v1/docs to list all API documentation.
func (s *Server) listDocs(w http.ResponseWriter, r *http.Request) {
	docs := s.docStore.List()
	writeJSON(w, http.StatusOK, docs)
}

// getDoc handles GET /api/v1/docs/{route} to get documentation for a specific route.
func (s *Server) getDoc(w http.ResponseWriter, r *http.Request) {
	routeName := r.PathValue("route")
	if routeName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route name is required"})
		return
	}

	doc, ok := s.docStore.Get(routeName)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "documentation for route '" + routeName + "' not found"})
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

// deleteDoc handles DELETE /api/v1/docs/{route} to unpublish documentation.
func (s *Server) deleteDoc(w http.ResponseWriter, r *http.Request) {
	routeName := r.PathValue("route")
	if routeName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route name is required"})
		return
	}

	if !s.docStore.Delete(routeName) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "documentation for route '" + routeName + "' not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "documentation unpublished successfully", "route_name": routeName})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
