package runtime

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/oriys/nexus/internal/config"
)

// Filter is the interface for request/response filters.
// Parameters are resolved at compile time, so Apply only does fast operations.
type Filter interface {
	Apply(r *http.Request) error
}

// FilterRegistry holds pre-compiled filter factories.
type FilterRegistry struct {
	factories map[string]FilterFactory
}

// FilterFactory creates a Filter from pre-parsed arguments.
type FilterFactory func(args map[string]string) (Filter, error)

// NewFilterRegistry creates a new FilterRegistry with built-in filters.
func NewFilterRegistry() *FilterRegistry {
	fr := &FilterRegistry{
		factories: make(map[string]FilterFactory),
	}
	fr.Register("strip_prefix", newStripPrefixFilter)
	fr.Register("header_set", newHeaderSetFilter)
	return fr
}

// Register registers a filter factory.
func (fr *FilterRegistry) Register(name string, factory FilterFactory) {
	fr.factories[name] = factory
}

// Compile compiles a RouteFilter DSL entry into a Filter.
func (fr *FilterRegistry) Compile(rf config.RouteFilter) (Filter, error) {
	factory, ok := fr.factories[rf.Type]
	if !ok {
		return nil, fmt.Errorf("unknown filter type: %s", rf.Type)
	}
	return factory(rf.Args)
}

// stripPrefixFilter removes a path prefix from the request URL.
type stripPrefixFilter struct {
	prefix string
}

func newStripPrefixFilter(args map[string]string) (Filter, error) {
	prefix := args["prefix"]
	if prefix == "" {
		return nil, fmt.Errorf("strip_prefix filter requires 'prefix' argument")
	}
	return &stripPrefixFilter{prefix: prefix}, nil
}

func (f *stripPrefixFilter) Apply(r *http.Request) error {
	if strings.HasPrefix(r.URL.Path, f.prefix) {
		newPath := strings.TrimPrefix(r.URL.Path, f.prefix)
		if newPath == "" || newPath[0] != '/' {
			newPath = "/" + newPath
		}
		r.URL.Path = newPath
		r.URL.RawPath = ""
	}
	return nil
}

// headerSetFilter sets a header on the request.
type headerSetFilter struct {
	key   string
	value string
}

func newHeaderSetFilter(args map[string]string) (Filter, error) {
	key := args["key"]
	value := args["value"]
	if key == "" {
		return nil, fmt.Errorf("header_set filter requires 'key' argument")
	}
	return &headerSetFilter{key: key, value: value}, nil
}

func (f *headerSetFilter) Apply(r *http.Request) error {
	r.Header.Set(f.key, f.value)
	return nil
}
