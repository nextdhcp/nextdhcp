package gotify

import "github.com/caddyserver/caddy"

func init() {
	caddy.RegisterPlugin("gotify", caddy.Plugin{
		ServerType: "dhcpv4",
		Action:     setupGotify,
	})
}

func setupGotify(c *caddy.Controller) error {
	return nil
}
