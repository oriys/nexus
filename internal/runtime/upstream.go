package runtime

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Upstream is the interface for protocol-specific upstream handlers.
type Upstream interface {
	Handle(w http.ResponseWriter, r *http.Request, route *CompiledRoute, cluster *CompiledCluster) error
}

// HTTPUpstream handles HTTP-to-HTTP proxying with streaming support.
type HTTPUpstream struct{}

// Handle proxies the request to the HTTP upstream using streaming reverse proxy.
func (u *HTTPUpstream) Handle(w http.ResponseWriter, r *http.Request, route *CompiledRoute, cluster *CompiledCluster) error {
	ep, ok := cluster.NextEndpoint()
	if !ok {
		return fmt.Errorf("no endpoints available for cluster %s", cluster.Name)
	}

	addr := EndpointAddress(ep)
	target, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid upstream target %s: %w", addr, err)
	}

	// If no scheme, default to http
	if target.Scheme == "" {
		target, err = url.Parse("http://" + addr)
		if err != nil {
			return fmt.Errorf("invalid upstream target %s: %w", addr, err)
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error",
				slog.String("cluster", cluster.Name),
				slog.String("target", addr),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	if route.TimeoutMs > 0 {
		proxy.FlushInterval = time.Duration(route.TimeoutMs) * time.Millisecond
	}

	proxy.ServeHTTP(w, r)
	return nil
}

// GRPCUpstream handles HTTP-to-gRPC proxying.
type GRPCUpstream struct{}

// Handle proxies the request to the gRPC upstream.
func (u *GRPCUpstream) Handle(w http.ResponseWriter, r *http.Request, route *CompiledRoute, cluster *CompiledCluster) error {
	grpcCfg := route.Upstream.GRPC
	if grpcCfg == nil {
		return fmt.Errorf("route %s missing gRPC upstream config", route.Name)
	}

	ep, ok := cluster.NextEndpoint()
	if !ok {
		return fmt.Errorf("no endpoints available for cluster %s", cluster.Name)
	}

	addr := EndpointAddress(ep)
	target, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid upstream target %s: %w", addr, err)
	}
	if target.Scheme == "" {
		target, err = url.Parse("http://" + addr)
		if err != nil {
			return fmt.Errorf("invalid upstream target %s: %w", addr, err)
		}
	}

	// Set gRPC path: /<service>/<method>
	r.URL.Path = "/" + grpcCfg.Service + "/" + grpcCfg.Method
	r.URL.RawPath = ""

	// Set gRPC content-type
	r.Header.Set("Content-Type", "application/grpc+json")

	// gRPC requires HTTP/2
	r.ProtoMajor = 2
	r.ProtoMinor = 0

	// Wrap body in gRPC length-prefixed framing if body exists
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		var framedBuf bytes.Buffer
		framedBuf.WriteByte(0) // not compressed
		msgLen := make([]byte, 4)
		binary.BigEndian.PutUint32(msgLen, uint32(len(bodyBytes)))
		framedBuf.Write(msgLen)
		framedBuf.Write(bodyBytes)

		r.Body = io.NopCloser(&framedBuf)
		r.ContentLength = int64(framedBuf.Len())
	}

	r.Header.Set("TE", "trailers")

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if cluster.GRPC != nil && cluster.GRPC.Authority != "" {
				pr.Out.Host = cluster.GRPC.Authority
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("grpc proxy error",
				slog.String("cluster", cluster.Name),
				slog.String("target", addr),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
	return nil
}

// dubboInvocation represents a Dubbo invocation request.
type dubboInvocation struct {
	Interface  string      `json:"interface"`
	Method     string      `json:"method"`
	ParamTypes []string    `json:"param_types,omitempty"`
	Args       interface{} `json:"args"`
}

// DubboUpstream handles HTTP-to-Dubbo proxying.
type DubboUpstream struct{}

// Handle proxies the request to the Dubbo upstream.
func (u *DubboUpstream) Handle(w http.ResponseWriter, r *http.Request, route *CompiledRoute, cluster *CompiledCluster) error {
	dubboCfg := route.Upstream.Dubbo
	if dubboCfg == nil {
		return fmt.Errorf("route %s missing Dubbo upstream config", route.Name)
	}

	ep, ok := cluster.NextEndpoint()
	if !ok {
		return fmt.Errorf("no endpoints available for cluster %s", cluster.Name)
	}

	addr := EndpointAddress(ep)
	target, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid upstream target %s: %w", addr, err)
	}
	if target.Scheme == "" {
		target, err = url.Parse("http://" + addr)
		if err != nil {
			return fmt.Errorf("invalid upstream target %s: %w", addr, err)
		}
	}

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
				args = string(bodyBytes)
			}
		}
	}

	// Build the Dubbo invocation request
	inv := dubboInvocation{
		Interface:  dubboCfg.Interface,
		Method:     dubboCfg.Method,
		ParamTypes: dubboCfg.ParamTypes,
		Args:       args,
	}

	encoded, err := json.Marshal(inv)
	if err != nil {
		return fmt.Errorf("failed to encode dubbo invocation: %w", err)
	}

	// Set the path for Dubbo triple protocol
	r.URL.Path = "/" + dubboCfg.Interface + "/" + dubboCfg.Method
	r.URL.RawPath = ""

	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	r.Header.Set("Content-Type", "application/json")
	r.Method = http.MethodPost

	// Set Dubbo-specific headers from cluster config
	if cluster.Dubbo != nil {
		if cluster.Dubbo.Group != "" {
			r.Header.Set("Dubbo-Group", cluster.Dubbo.Group)
		}
		if cluster.Dubbo.Version != "" {
			r.Header.Set("Dubbo-Version", cluster.Dubbo.Version)
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("dubbo proxy error",
				slog.String("cluster", cluster.Name),
				slog.String("target", addr),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
	return nil
}

// UpstreamDispatcher dispatches requests to the appropriate upstream handler based on cluster type.
type UpstreamDispatcher struct {
	httpUpstream  *HTTPUpstream
	grpcUpstream  *GRPCUpstream
	dubboUpstream *DubboUpstream
}

// NewUpstreamDispatcher creates a new UpstreamDispatcher.
func NewUpstreamDispatcher() *UpstreamDispatcher {
	return &UpstreamDispatcher{
		httpUpstream:  &HTTPUpstream{},
		grpcUpstream:  &GRPCUpstream{},
		dubboUpstream: &DubboUpstream{},
	}
}

// Dispatch routes the request to the appropriate upstream handler.
func (d *UpstreamDispatcher) Dispatch(w http.ResponseWriter, r *http.Request, route *CompiledRoute, cluster *CompiledCluster) error {
	switch cluster.Type {
	case "grpc":
		return d.grpcUpstream.Handle(w, r, route, cluster)
	case "dubbo":
		return d.dubboUpstream.Handle(w, r, route, cluster)
	default:
		return d.httpUpstream.Handle(w, r, route, cluster)
	}
}
