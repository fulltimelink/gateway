package server

import (
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/go-kratos/kratos/v2/middleware/metrics"

	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/prometheus/client_golang/prometheus"

	prom "github.com/go-kratos/kratos/contrib/metrics/prometheus/v2"
)

var (
	// --  @# 追加metrics
	_metricRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "client",
		Subsystem: "requests",
		Name:      "code_total",
		Help:      "The total number of processed requests",
	}, []string{"kind", "operation", "code", "reason"})
)

func init() {
	prometheus.MustRegister(_metricRequests)
}

// NewMetrics new a metrics collection server.
func NewMetrics() *http.Server {
	opts := []http.ServerOption{
		http.Middleware(
			metrics.Server(
				metrics.WithRequests(prom.NewCounter(_metricRequests)),
			),
		),
		http.Network("tcp"),
		http.Address(":6060"),
		http.Timeout(3 * time.Second),
	}
	srv := http.NewServer(opts...)
	srv.Handle("/metrics", promhttp.Handler())
	return srv
}
