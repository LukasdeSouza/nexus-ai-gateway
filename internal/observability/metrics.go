package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric vectors for the gateway.
type Metrics struct {
	HTTPRequestsTotal        *prometheus.CounterVec
	HTTPRequestDuration      *prometheus.HistogramVec
	ProviderRequestsTotal    *prometheus.CounterVec
	ProviderRequestDuration  *prometheus.HistogramVec
	ProviderFallbackTotal    *prometheus.CounterVec
	LLMTokensTotal           *prometheus.CounterVec
	LLMEstimatedCostTotal    *prometheus.CounterVec
	RateLimitRejectionsTotal *prometheus.CounterVec
}

var defaultRegistry *prometheus.Registry

func init() {
	defaultRegistry = NewPrometheusRegistry()
}

// NewPrometheusRegistry creates a registry with Go and process collectors.
func NewPrometheusRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// PrometheusHandler returns an http.Handler serving the default registry.
func PrometheusHandler() http.Handler {
	return promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{})
}

// NewMetrics initializes and registers all gateway Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = defaultRegistry
	}

	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests processed by the gateway.",
			},
			[]string{"service", "route", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Latency of HTTP requests processed by the gateway in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "route"},
		),
		ProviderRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "provider_requests_total",
				Help: "Total requests dispatched to upstream AI providers.",
			},
			[]string{"provider", "model", "status"},
		),
		ProviderRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "provider_request_duration_seconds",
				Help:    "Latency of upstream AI provider requests in seconds.",
				Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
			},
			[]string{"provider", "model"},
		),
		ProviderFallbackTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "provider_fallback_total",
				Help: "Total number of provider failover occurrences.",
			},
			[]string{"provider", "target_provider"},
		),
		LLMTokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_tokens_total",
				Help: "Total LLM tokens processed categorized by direction (input/output).",
			},
			[]string{"direction", "provider", "model"},
		),
		LLMEstimatedCostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "llm_estimated_cost_usd_total",
				Help: "Estimated cost in USD accumulated across model providers.",
			},
			[]string{"provider", "model"},
		),
		RateLimitRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limit_rejections_total",
				Help: "Total requests rejected due to rate limits.",
			},
			[]string{"scope"},
		),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.ProviderRequestsTotal,
		m.ProviderRequestDuration,
		m.ProviderFallbackTotal,
		m.LLMTokensTotal,
		m.LLMEstimatedCostTotal,
		m.RateLimitRejectionsTotal,
	)

	return m
}
