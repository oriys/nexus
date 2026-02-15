package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nexus/internal/config"
)

func TestApplyRewrite_NilRewrite(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/v1/users", nil)
	route := config.Route{Name: "test", Upstream: "backend"}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.Path != "/api/v1/users" {
		t.Errorf("path should be unchanged, got %s", req.URL.Path)
	}
}

func TestApplyHTTPRewrite_PathRewrite(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/v1/users", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			PathRewrite: &config.PathRewrite{
				Prefix: "/internal",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.Path != "/internal/v1/users" {
		t.Errorf("expected /internal/v1/users, got %s", req.URL.Path)
	}
}

func TestApplyHTTPRewrite_PathRewriteRoot(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			PathRewrite: &config.PathRewrite{
				Prefix: "/v2",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.Path != "/v2" {
		t.Errorf("expected /v2, got %s", req.URL.Path)
	}
}

func TestApplyHTTPRewrite_PathRewriteEmptyPrefix(t *testing.T) {
	// When PathRewrite prefix is empty, no path rewrite is performed
	req := httptest.NewRequest("GET", "http://example.com/api/v1", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			PathRewrite: &config.PathRewrite{
				Prefix: "",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Path should remain unchanged since prefix is empty
	if req.URL.Path != "/api/v1" {
		t.Errorf("expected /api/v1 (unchanged), got %s", req.URL.Path)
	}
}

func TestApplyHTTPRewrite_HeaderAdd(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			Headers: &config.HeaderRewrite{
				Add: map[string]string{
					"X-Custom": "value1",
				},
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Header.Get("X-Custom") != "value1" {
		t.Errorf("expected X-Custom=value1, got %s", req.Header.Get("X-Custom"))
	}
}

func TestApplyHTTPRewrite_HeaderSet(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	req.Header.Set("X-Existing", "old")
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			Headers: &config.HeaderRewrite{
				Set: map[string]string{
					"X-Existing": "new",
				},
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Header.Get("X-Existing") != "new" {
		t.Errorf("expected X-Existing=new, got %s", req.Header.Get("X-Existing"))
	}
}

func TestApplyHTTPRewrite_HeaderRemove(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	req.Header.Set("X-Remove-Me", "value")
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			Headers: &config.HeaderRewrite{
				Remove: []string{"X-Remove-Me"},
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Header.Get("X-Remove-Me") != "" {
		t.Errorf("expected X-Remove-Me to be removed, got %s", req.Header.Get("X-Remove-Me"))
	}
}

func TestApplyHTTPRewrite_CombinedPathAndHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/v1/users", nil)
	req.Header.Set("X-Old", "old")
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "http",
			PathRewrite: &config.PathRewrite{
				Prefix: "/internal",
			},
			Headers: &config.HeaderRewrite{
				Add:    map[string]string{"X-New": "new"},
				Set:    map[string]string{"X-Old": "updated"},
				Remove: []string{"X-Remove"},
			},
		},
	}
	req.Header.Set("X-Remove", "removeme")

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.Path != "/internal/v1/users" {
		t.Errorf("expected /internal/v1/users, got %s", req.URL.Path)
	}
	if req.Header.Get("X-New") != "new" {
		t.Errorf("expected X-New=new, got %s", req.Header.Get("X-New"))
	}
	if req.Header.Get("X-Old") != "updated" {
		t.Errorf("expected X-Old=updated, got %s", req.Header.Get("X-Old"))
	}
	if req.Header.Get("X-Remove") != "" {
		t.Errorf("expected X-Remove to be removed")
	}
}

func TestApplyHTTPRewrite_DefaultProtocol(t *testing.T) {
	// When protocol is empty, should default to "http"
	req := httptest.NewRequest("GET", "http://example.com/api/v1/users", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			PathRewrite: &config.PathRewrite{
				Prefix: "/internal",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.Path != "/internal/v1/users" {
		t.Errorf("expected /internal/v1/users, got %s", req.URL.Path)
	}
}

func TestApplyGRPCRewrite(t *testing.T) {
	body := `{"name": "world"}`
	req := httptest.NewRequest("POST", "http://example.com/api/hello", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	route := config.Route{
		Name:     "grpc-route",
		Upstream: "grpc-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "grpc",
			GRPC: &config.GRPCRewrite{
				Service: "helloworld.Greeter",
				Method:  "SayHello",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check path
	expectedPath := "/helloworld.Greeter/SayHello"
	if req.URL.Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, req.URL.Path)
	}

	// Check content-type
	if req.Header.Get("Content-Type") != "application/grpc+json" {
		t.Errorf("expected content-type application/grpc+json, got %s", req.Header.Get("Content-Type"))
	}

	// Check TE header
	if req.Header.Get("TE") != "trailers" {
		t.Errorf("expected TE=trailers, got %s", req.Header.Get("TE"))
	}

	// Check HTTP/2
	if req.ProtoMajor != 2 {
		t.Errorf("expected HTTP/2, got HTTP/%d.%d", req.ProtoMajor, req.ProtoMinor)
	}

	// Read and verify gRPC-framed body
	framedBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if len(framedBody) < 5 {
		t.Fatalf("framed body too short: %d bytes", len(framedBody))
	}

	// First byte: compressed flag (0 = not compressed)
	if framedBody[0] != 0 {
		t.Errorf("expected compressed flag 0, got %d", framedBody[0])
	}

	// Next 4 bytes: message length
	msgLen := binary.BigEndian.Uint32(framedBody[1:5])
	if int(msgLen) != len(body) {
		t.Errorf("expected message length %d, got %d", len(body), msgLen)
	}

	// Rest: the original JSON body
	msgBody := string(framedBody[5:])
	if msgBody != body {
		t.Errorf("expected body %q, got %q", body, msgBody)
	}
}

func TestApplyGRPCRewrite_WithHeaders(t *testing.T) {
	body := `{"name": "world"}`
	req := httptest.NewRequest("POST", "http://example.com/api/hello", strings.NewReader(body))

	route := config.Route{
		Name:     "grpc-route",
		Upstream: "grpc-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "grpc",
			GRPC: &config.GRPCRewrite{
				Service: "helloworld.Greeter",
				Method:  "SayHello",
			},
			Headers: &config.HeaderRewrite{
				Add: map[string]string{"X-Custom-Header": "grpc-value"},
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Header.Get("X-Custom-Header") != "grpc-value" {
		t.Errorf("expected X-Custom-Header=grpc-value, got %s", req.Header.Get("X-Custom-Header"))
	}
}

func TestApplyGRPCRewrite_NilBody(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/hello", nil)

	route := config.Route{
		Name:     "grpc-route",
		Upstream: "grpc-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "grpc",
			GRPC: &config.GRPCRewrite{
				Service: "helloworld.Greeter",
				Method:  "SayHello",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := "/helloworld.Greeter/SayHello"
	if req.URL.Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, req.URL.Path)
	}
}

func TestApplyDubboRewrite(t *testing.T) {
	body := `{"userId": 123}`
	req := httptest.NewRequest("POST", "http://example.com/api/user/get", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	route := config.Route{
		Name:     "dubbo-route",
		Upstream: "dubbo-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "dubbo",
			Dubbo: &config.DubboRewrite{
				Service: "com.example.UserService",
				Method:  "getUser",
				Group:   "default",
				Version: "1.0.0",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/user/get")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check path
	expectedPath := "/com.example.UserService/getUser"
	if req.URL.Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, req.URL.Path)
	}

	// Check content-type
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type application/json, got %s", req.Header.Get("Content-Type"))
	}

	// Check Dubbo headers
	if req.Header.Get("Dubbo-Group") != "default" {
		t.Errorf("expected Dubbo-Group=default, got %s", req.Header.Get("Dubbo-Group"))
	}
	if req.Header.Get("Dubbo-Version") != "1.0.0" {
		t.Errorf("expected Dubbo-Version=1.0.0, got %s", req.Header.Get("Dubbo-Version"))
	}

	// Check method is POST
	if req.Method != http.MethodPost {
		t.Errorf("expected POST method, got %s", req.Method)
	}

	// Read and verify the Dubbo request body
	respBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var dubboReq dubboRequest
	if err := json.Unmarshal(respBody, &dubboReq); err != nil {
		t.Fatalf("failed to unmarshal dubbo request: %v", err)
	}

	if dubboReq.Service != "com.example.UserService" {
		t.Errorf("expected service com.example.UserService, got %s", dubboReq.Service)
	}
	if dubboReq.Method != "getUser" {
		t.Errorf("expected method getUser, got %s", dubboReq.Method)
	}
	if dubboReq.Group != "default" {
		t.Errorf("expected group default, got %s", dubboReq.Group)
	}
	if dubboReq.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", dubboReq.Version)
	}
}

func TestApplyDubboRewrite_NilBody(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/user/list", nil)

	route := config.Route{
		Name:     "dubbo-route",
		Upstream: "dubbo-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "dubbo",
			Dubbo: &config.DubboRewrite{
				Service: "com.example.UserService",
				Method:  "listUsers",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/user/list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := "/com.example.UserService/listUsers"
	if req.URL.Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, req.URL.Path)
	}

	// Check no Dubbo-Group/Dubbo-Version headers since they are empty
	if req.Header.Get("Dubbo-Group") != "" {
		t.Errorf("expected no Dubbo-Group header, got %s", req.Header.Get("Dubbo-Group"))
	}
	if req.Header.Get("Dubbo-Version") != "" {
		t.Errorf("expected no Dubbo-Version header, got %s", req.Header.Get("Dubbo-Version"))
	}
}

func TestApplyDubboRewrite_WithHeaders(t *testing.T) {
	body := `{"id": 1}`
	req := httptest.NewRequest("POST", "http://example.com/api/user", strings.NewReader(body))

	route := config.Route{
		Name:     "dubbo-route",
		Upstream: "dubbo-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "dubbo",
			Dubbo: &config.DubboRewrite{
				Service: "com.example.UserService",
				Method:  "getUser",
			},
			Headers: &config.HeaderRewrite{
				Add: map[string]string{"X-Trace-ID": "abc123"},
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Header.Get("X-Trace-ID") != "abc123" {
		t.Errorf("expected X-Trace-ID=abc123, got %s", req.Header.Get("X-Trace-ID"))
	}
}

func TestApplyDubboRewrite_NonJSONBody(t *testing.T) {
	body := "plain text body"
	req := httptest.NewRequest("POST", "http://example.com/api/user", strings.NewReader(body))

	route := config.Route{
		Name:     "dubbo-route",
		Upstream: "dubbo-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "dubbo",
			Dubbo: &config.DubboRewrite{
				Service: "com.example.UserService",
				Method:  "process",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read the body and verify the args field is a string
	respBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var dubboReq dubboRequest
	if err := json.Unmarshal(respBody, &dubboReq); err != nil {
		t.Fatalf("failed to unmarshal dubbo request: %v", err)
	}

	if dubboReq.Args != "plain text body" {
		t.Errorf("expected args='plain text body', got %v", dubboReq.Args)
	}
}

func TestApplyRewrite_UnsupportedProtocol(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	route := config.Route{
		Name:     "test",
		Upstream: "backend",
		Rewrite: &config.RewriteRule{
			Protocol: "websocket",
		},
	}

	err := ApplyRewrite(req, route, "/api")
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if !strings.Contains(err.Error(), "unsupported rewrite protocol") {
		t.Errorf("expected 'unsupported rewrite protocol' in error, got %s", err.Error())
	}
}

func TestFindMatchedPath_ExactMatch(t *testing.T) {
	route := config.Route{
		Paths: []config.PathRule{
			{Path: "/api/v1", Type: "exact"},
		},
	}

	matched := findMatchedPath(route, "/api/v1")
	if matched != "/api/v1" {
		t.Errorf("expected /api/v1, got %s", matched)
	}
}

func TestFindMatchedPath_PrefixMatch(t *testing.T) {
	route := config.Route{
		Paths: []config.PathRule{
			{Path: "/api", Type: "prefix"},
		},
	}

	matched := findMatchedPath(route, "/api/v1/users")
	if matched != "/api" {
		t.Errorf("expected /api, got %s", matched)
	}
}

func TestFindMatchedPath_NoMatch(t *testing.T) {
	route := config.Route{
		Paths: []config.PathRule{
			{Path: "/api", Type: "exact"},
		},
	}

	matched := findMatchedPath(route, "/web")
	if matched != "" {
		t.Errorf("expected empty string, got %s", matched)
	}
}

func TestRouterMatch_ReturnsRouteWithRewrite(t *testing.T) {
	router := NewRouter()
	rewrite := &config.RewriteRule{
		Protocol: "http",
		PathRewrite: &config.PathRewrite{
			Prefix: "/internal",
		},
	}

	router.Reload([]config.Route{
		{
			Name:     "api",
			Host:     "",
			Upstream: "backend",
			Paths:    []config.PathRule{{Path: "/api", Type: "prefix"}},
			Rewrite:  rewrite,
		},
	})

	req := httptest.NewRequest("GET", "http://localhost/api/v1/users", nil)
	result, ok := router.Match(req)
	if !ok {
		t.Fatal("expected match")
	}
	if result.Upstream != "backend" {
		t.Errorf("expected backend, got %s", result.Upstream)
	}
	if result.Route.Rewrite == nil {
		t.Fatal("expected rewrite to be present")
	}
	if result.Route.Rewrite.Protocol != "http" {
		t.Errorf("expected protocol http, got %s", result.Route.Rewrite.Protocol)
	}
	if result.Route.Rewrite.PathRewrite.Prefix != "/internal" {
		t.Errorf("expected path rewrite prefix /internal, got %s", result.Route.Rewrite.PathRewrite.Prefix)
	}
}

func TestApplyGRPCRewrite_EmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/api/hello", bytes.NewReader([]byte{}))

	route := config.Route{
		Name:     "grpc-route",
		Upstream: "grpc-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "grpc",
			GRPC: &config.GRPCRewrite{
				Service: "helloworld.Greeter",
				Method:  "SayHello",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read the framed body
	framedBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	// Should have 5-byte frame header + 0 bytes of body
	if len(framedBody) != 5 {
		t.Errorf("expected 5 bytes (frame header only), got %d", len(framedBody))
	}

	// Message length should be 0
	msgLen := binary.BigEndian.Uint32(framedBody[1:5])
	if msgLen != 0 {
		t.Errorf("expected message length 0, got %d", msgLen)
	}
}

func TestApplyDubboRewrite_EmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/api/user", bytes.NewReader([]byte{}))

	route := config.Route{
		Name:     "dubbo-route",
		Upstream: "dubbo-backend",
		Rewrite: &config.RewriteRule{
			Protocol: "dubbo",
			Dubbo: &config.DubboRewrite{
				Service: "com.example.UserService",
				Method:  "listAll",
			},
		},
	}

	err := ApplyRewrite(req, route, "/api/user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read the body
	respBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	var dubboReq dubboRequest
	if err := json.Unmarshal(respBody, &dubboReq); err != nil {
		t.Fatalf("failed to unmarshal dubbo request: %v", err)
	}

	if dubboReq.Args != nil {
		t.Errorf("expected nil args, got %v", dubboReq.Args)
	}
}
