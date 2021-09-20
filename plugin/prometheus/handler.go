package prometheus

import (
	"context"
	"log"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// Name return the name of plugin
func (p *Plugin) Name() string {
	return "prometheus"
}

//ServeDHCP server dhcp request and response
func (p *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	var extraLabelValues []string

	for _, label := range p.Metrics.extraLabels {
		extraLabelValues = append(extraLabelValues, label.value)
	}
	start := time.Now()
	err := p.Next.ServeDHCP(ctx, req, res)

	requestType := req.MessageType().String()
	responseType := res.MessageType().String()
	requestDuration.WithLabelValues(append([]string{requestType, responseType}, extraLabelValues...)...).Observe(float64(time.Since(start).Seconds()))
	requestCount.WithLabelValues(append([]string{requestType}, extraLabelValues...)...).Inc()
	log.Printf("Prometheus monitoring request_type: %s, response_type: %s", requestType, responseType)

	return err
}
