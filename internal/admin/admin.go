package admin

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nexus/internal/config"
	"github.com/oriys/nexus/internal/proxy"
)

// Server is the admin API server.
type Server struct {
	configLoader   *config.Loader
	versionManager *config.VersionManager
	router         *proxy.Router
	upstreamMgr    *proxy.UpstreamManager
	mux            *http.ServeMux
}

// New creates a new admin server and registers routes.
func New(cl *config.Loader, vm *config.VersionManager, r *proxy.Router, um *proxy.UpstreamManager) *Server {
	s := &Server{
		configLoader:   cl,
		versionManager: vm,
		router:         r,
		upstreamMgr:    um,
		mux:            http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /api/v1/config", s.getConfig)
	s.mux.HandleFunc("GET /api/v1/config/versions", s.listVersions)
	s.mux.HandleFunc("POST /api/v1/config/rollback", s.rollbackConfig)
	s.mux.HandleFunc("GET /api/v1/routes", s.listRoutes)
	s.mux.HandleFunc("GET /api/v1/upstreams", s.listUpstreams)
	s.mux.HandleFunc("GET /api/v1/status", s.getStatus)
	return s
}

// Handler returns the HTTP handler for the admin server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configLoader.Current()
	if cfg == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "no configuration loaded"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	versions := s.versionManager.List()
	type versionInfo struct {
		Version   int    `json:"version"`
		Hash      string `json:"hash"`
		Timestamp string `json:"timestamp"`
	}
	result := make([]versionInfo, len(versions))
	for i, v := range versions {
		result[i] = versionInfo{
			Version:   v.Version,
			Hash:      v.Hash,
			Timestamp: v.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) rollbackConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.versionManager.Rollback()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	s.router.Reload(cfg.Routes)
	s.upstreamMgr.Reload(cfg.Upstreams)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "configuration rolled back successfully"})
}

func (s *Server) listRoutes(w http.ResponseWriter, r *http.Request) {
	cfg := s.configLoader.Current()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg.Routes)
}

func (s *Server) listUpstreams(w http.ResponseWriter, r *http.Request) {
	cfg := s.configLoader.Current()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cfg.Upstreams)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "running",
		"config_versions": s.versionManager.Len(),
	})
}
