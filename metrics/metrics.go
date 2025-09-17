package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ActiveConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "akash_active_connections",
		Help: "Number of active connections",
	})

	PerBackendServed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "akash_backend_served_total",
			Help: "Total requests successfully served per backend",
		},
		[]string{"backend"},
	)

	PerBackendFails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "akash_backend_failures_total",
			Help: "Total failures per backend",
		},
		[]string{"backend"},
	)
)

func StartMetricsServer(addr string) {
	reg := prometheus.NewRegistry()

	reg.MustRegister(ActiveConns, PerBackendServed, PerBackendFails)

	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		log.Printf("Prometheus metrics available at %s/metrics", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("Prometheus metrics server error: %v", err)
		}
	}()
}
