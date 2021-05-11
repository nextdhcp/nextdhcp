package monitor

import (
	"log"
	"net/http"
	"sync"

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

type Metrics struct {
	addr           string // where to we listen
	hostname       string
	path           string
	extraLabels    []extraLabel
	latencyBuckets []float64

	once sync.Once

	handler http.Handler
}

type extraLabel struct {
	name  string
	value string
}

// NewMetrics -
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
		Name: "request_count_total",
		Help: "Counter of DHCP(S) requests made.",
	}, append([]string{"host", "request_type"}, extraLabels...))

	requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "request_duration_seconds",
		Help:    "Histogram of the time (in seconds) each request took.",
		Buckets: m.latencyBuckets,
	}, append([]string{"host", "request_type", "responce_type"}, extraLabels...))
}

func (m *Metrics) start() error {
	m.once.Do(func() {
		m.define()

		prometheus.MustRegister(requestCount)
		prometheus.MustRegister(requestDuration)

		http.Handle(m.path, promhttp.Handler())
		go func() {
			err := http.ListenAndServe(m.addr, m.handler)
			if err != nil {
				log.Printf("[ERROR] Starting handler: %v", err)
			}
		}()

	})
	return nil
}
