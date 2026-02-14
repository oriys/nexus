package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// RequestsTotal counts the total number of HTTP requests.
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "nexus",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	// RequestDuration observes the request duration in seconds.
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "nexus",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// UpstreamHealthy tracks the health status of upstream targets.
	UpstreamHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nexus",
			Name:      "upstream_healthy",
			Help:      "Whether an upstream target is healthy (1) or not (0).",
		},
		[]string{"upstream", "target"},
	)
)

func init() {
	prometheus.MustRegister(RequestsTotal, RequestDuration, UpstreamHealthy)
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordRequest records metrics for a completed HTTP request.
func RecordRequest(method, path string, status int, duration time.Duration) {
	RequestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	RequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}
