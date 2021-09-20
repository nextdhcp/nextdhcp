package prometheus

import (
	"strconv"
	"strings"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/dhcpserver"
	"github.com/nextdhcp/nextdhcp/plugin"
)

func init() {
	caddy.RegisterPlugin("prometheus", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupPrometheus,
	})
}

// Plugin prometheus plugin
type Plugin struct {
	Next    plugin.Handler
	Metrics *Metrics
}

func setupPrometheus(c *caddy.Controller) error {
	plg, err := parse(c)
	if err != nil {
		return err
	}
	dhcpserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plg.Next = next
		return plg
	})

	return nil
}

func parse(c *caddy.Controller) (*Plugin, error) {
	var (
		metrics *Metrics
		err     error
	)
	p := &Plugin{}

	for c.Next() {
		if metrics != nil {
			return nil, c.Err("prometheus: can only have one metrics module per server")
		}

		args := c.RemainingArgs()
		metrics = NewMetrics("", "")
		switch len(args) {
		case 0:
		case 1:
			metrics.addr = args[0]
		default:
			return nil, c.ArgErr()
		}
		for c.NextBlock() {
			switch c.Val() {
			case "path":
				args = c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				metrics.path = args[0]
			case "address":
				args = c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				metrics.addr = args[0]
			case "hostname":
				args = c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				metrics.hostname = args[0]
			case "label":
				args = c.RemainingArgs()
				if len(args) != 2 {
					return nil, c.ArgErr()
				}

				labelName := strings.TrimSpace(args[0])
				labelValuePlaceholder := args[1]

				metrics.extraLabels = append(metrics.extraLabels, extraLabel{name: labelName, value: labelValuePlaceholder})
			case "latency_buckets":
				args = c.RemainingArgs()
				if len(args) < 1 {
					return nil, c.Err("prometheus: must specify 1 or more latency buckets")
				}
				metrics.latencyBuckets = make([]float64, len(args))
				for i, v := range args {
					b, err := strconv.ParseFloat(v, 64)
					if err != nil {
						return nil, c.Errf("prometheus: invalid bucket %q - must be a number", v)
					}
					metrics.latencyBuckets[i] = b
				}
			default:
				return nil, c.Errf("prometheus: unknown item: %s", c.Val())
			}
		}
	}

	err = metrics.start()
	if err != nil {
		return nil, err
	}

	p.Metrics = metrics
	return p, err
}
