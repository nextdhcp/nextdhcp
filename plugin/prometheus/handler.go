package monitor

import (
	"context"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
)

func (p *Plugin) Name() string {
	return "prometheus"
}

func (p *Plugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	var extraLabelValues []string

	requestType := req.MessageType().String()
	responseType := res.MessageType().String()

	for _, label := range p.Metrics.extraLabels {
		extraLabelValues = append(extraLabelValues, label.value)
	}
	requestTimeStamp := dhcpserver.GetRequestTimeStamp(ctx)
	requestCount.WithLabelValues(append(extraLabelValues, requestType)...).Inc()
	requestDuration.WithLabelValues(append([]string{requestType, responseType}, extraLabelValues...)...).Observe(float64(time.Since(requestTimeStamp).Seconds()))
	return nil
}
