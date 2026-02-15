package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/oriys/nexus/internal/config"
)

// applyHTTPRewrite applies HTTP-to-HTTP request rewriting rules.
func applyHTTPRewrite(r *http.Request, rw *config.RewriteRule, matchedPath string) {
	if rw == nil {
		return
	}

	// Apply path rewrite
	if rw.PathRewrite != nil && rw.PathRewrite.Prefix != "" {
		originalPath := r.URL.Path
		if matchedPath != "" && strings.HasPrefix(originalPath, matchedPath) {
			r.URL.Path = rw.PathRewrite.Prefix + strings.TrimPrefix(originalPath, matchedPath)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			r.URL.RawPath = ""
		}
	}

	// Apply header rewrites
	applyHeaderRewrite(r, rw.Headers)
}

// applyHeaderRewrite applies header manipulation rules to the request.
func applyHeaderRewrite(r *http.Request, headers *config.HeaderRewrite) {
	if headers == nil {
		return
	}

	// Add headers (append)
	for key, value := range headers.Add {
		r.Header.Add(key, value)
	}

	// Set headers (overwrite)
	for key, value := range headers.Set {
		r.Header.Set(key, value)
	}

	// Remove headers
	for _, key := range headers.Remove {
		r.Header.Del(key)
	}
}

// applyGRPCRewrite transforms an HTTP request into a gRPC-compatible request.
// It sets the appropriate path, content-type, and wraps the JSON body in gRPC framing.
func applyGRPCRewrite(r *http.Request, rw *config.RewriteRule) error {
	if rw == nil || rw.GRPC == nil {
		return nil
	}

	grpc := rw.GRPC

	// Set gRPC path: /<service>/<method>
	r.URL.Path = "/" + grpc.Service + "/" + grpc.Method
	r.URL.RawPath = ""

	// Set gRPC content-type
	r.Header.Set("Content-Type", "application/grpc+json")

	// gRPC requires HTTP/2
	r.ProtoMajor = 2
	r.ProtoMinor = 0

	// Read the JSON body and wrap in gRPC length-prefixed framing
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		// gRPC framing: 1 byte compressed flag + 4 bytes message length + message
		var framedBuf bytes.Buffer
		framedBuf.WriteByte(0) // not compressed
		msgLen := make([]byte, 4)
		binary.BigEndian.PutUint32(msgLen, uint32(len(bodyBytes)))
		framedBuf.Write(msgLen)
		framedBuf.Write(bodyBytes)

		r.Body = io.NopCloser(&framedBuf)
		r.ContentLength = int64(framedBuf.Len())
	}

	// Set additional gRPC headers
	r.Header.Set("TE", "trailers")

	// Apply additional header rewrites if specified
	applyHeaderRewrite(r, rw.Headers)

	return nil
}

// dubboRequest represents a simplified Dubbo invocation request encoded as JSON.
type dubboRequest struct {
	Service string      `json:"service"`
	Method  string      `json:"method"`
	Group   string      `json:"group,omitempty"`
	Version string      `json:"version,omitempty"`
	Args    interface{} `json:"args"`
}

// applyDubboRewrite transforms an HTTP request into a Dubbo-compatible HTTP request.
// Since Dubbo 3.x supports triple protocol (HTTP/2-based), this creates a JSON-encoded
// Dubbo invocation payload that can be proxied to a Dubbo gateway or triple protocol endpoint.
func applyDubboRewrite(r *http.Request, rw *config.RewriteRule) error {
	if rw == nil || rw.Dubbo == nil {
		return nil
	}

	dubbo := rw.Dubbo

	// Read original body as the method arguments
	var args interface{}
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		if len(bodyBytes) > 0 {
			if err := json.Unmarshal(bodyBytes, &args); err != nil {
				// If not valid JSON, pass as raw string
				args = string(bodyBytes)
			}
		}
	}

	// Build the Dubbo invocation request
	dubboReq := dubboRequest{
		Service: dubbo.Service,
		Method:  dubbo.Method,
		Group:   dubbo.Group,
		Version: dubbo.Version,
		Args:    args,
	}

	// Encode as JSON
	encoded, err := json.Marshal(dubboReq)
	if err != nil {
		return fmt.Errorf("failed to encode dubbo request: %w", err)
	}

	// Set the path for Dubbo triple protocol
	r.URL.Path = "/" + dubbo.Service + "/" + dubbo.Method
	r.URL.RawPath = ""

	// Set appropriate content type and body
	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	r.Header.Set("Content-Type", "application/json")

	// Set Dubbo-specific headers
	if dubbo.Group != "" {
		r.Header.Set("Dubbo-Group", dubbo.Group)
	}
	if dubbo.Version != "" {
		r.Header.Set("Dubbo-Version", dubbo.Version)
	}

	r.Method = http.MethodPost

	// Apply additional header rewrites if specified
	applyHeaderRewrite(r, rw.Headers)

	return nil
}

// ApplyRewrite applies the appropriate rewrite based on the route's protocol configuration.
// It returns an error if the rewrite fails.
func ApplyRewrite(r *http.Request, route config.Route, matchedPath string) error {
	rw := route.Rewrite
	if rw == nil {
		return nil
	}

	protocol := rw.Protocol
	if protocol == "" {
		protocol = "http"
	}

	switch protocol {
	case "http":
		applyHTTPRewrite(r, rw, matchedPath)
		return nil
	case "grpc":
		if err := applyGRPCRewrite(r, rw); err != nil {
			slog.Error("grpc rewrite failed",
				slog.String("route", route.Name),
				slog.String("error", err.Error()),
			)
			return err
		}
		return nil
	case "dubbo":
		if err := applyDubboRewrite(r, rw); err != nil {
			slog.Error("dubbo rewrite failed",
				slog.String("route", route.Name),
				slog.String("error", err.Error()),
			)
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported rewrite protocol: %s", protocol)
	}
}
