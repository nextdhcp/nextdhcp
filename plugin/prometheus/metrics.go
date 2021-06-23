package prometheus

import (
	"net/http"
	"sync"

	"github.com/apex/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultPath = "/metrics"
	defaultAddr = "localhost:9180"
)

var (
	requestCount    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	once            sync.Once
)

// Metrics represents prometheus metrics
type Metrics struct {
	addr           string // where to we listen
	hostname       string
	path           string
	extraLabels    []extraLabel
	latencyBuckets []float64
}

type extraLabel struct {
	name  string
	value string
}

// NewMetrics create a new Metrics
func NewMetrics(path, addr string) *Metrics {

	p := path
	if path == "" {
		p = defaultPath
	}
	a := addr
	if addr == "" {
		a = defaultAddr
	}
	return &Metrics{
		path:        p,
		addr:        a,
		extraLabels: []extraLabel{},
	}
}

func (m *Metrics) extraLabelNames() []string {
	names := make([]string, 0, len(m.extraLabels))

	for _, label := range m.extraLabels {
		names = append(names, label.name)
	}

	return names
}

func (m *Metrics) define() {
	if m.latencyBuckets == nil {
		m.latencyBuckets = append(prometheus.DefBuckets, 15, 20, 30, 60, 120, 180, 240, 480, 960)
	}

	extraLabels := m.extraLabelNames()

	requestCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dhcp_request_count_total",
		Help: "Counter of DHCP(S) requests made.",
	}, append([]string{"request_type"}, extraLabels...))

	requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dhcp_request_duration_seconds",
		Help:    "Histogram of the time (in seconds) each request took.",
		Buckets: m.latencyBuckets,
	}, append([]string{"request_type", "response_type"}, extraLabels...))
}

func (m *Metrics) start() error {
	m.define()

	prometheus.MustRegister(requestCount)
	prometheus.MustRegister(requestDuration)

	// http.Handle(m.path, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, m.handler))
	http.Handle(m.path, promhttp.Handler())
	go func() {
		err := http.ListenAndServe(m.addr, nil)
		if err != nil {
			log.Errorf("[ERROR] Starting handler: %v", err)
		}

	}()

	return nil
}
