package gotify

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gotify/go-api-client/v2/auth"
	"github.com/gotify/go-api-client/v2/client/message"
	"github.com/gotify/go-api-client/v2/gotify"
	"github.com/gotify/go-api-client/v2/models"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/log"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/plugin"
)

type (
	// msgFactory creates the gotify notification message
	// from the given request and response DHCPv4 messages
	msgFactory func(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, error)

	// gotifyPlugin matches requests against a set of conditions
	// and sends notifications. It implements the plugin.Handler
	// interface
	gotifyPlugin struct {
		next          plugin.Handler
		notifications []*notification
		l             log.Logger
	}

	// notification combines the matcher (condition) and a message
	// factory for a gotify notification
	notification struct {
		*matcher.Matcher
		msg   msgFactory
		title msgFactory
		srv   string
		token string
	}
)

// Prepare checks if we should send a notification for the given request and returns
// the message body. An empty message body indicates that no notification should be
// sent
func (n *notification) Prepare(ctx context.Context, req, res *dhcpv4.DHCPv4) (string, string, error) {
	if n.msg == nil {
		return "", "", nil
	}

	matched, err := n.Match(ctx, req)
	if err != nil {
		return "", "", err
	}

	if matched {
		msg, err := n.msg(ctx, req, res)
		if err != nil {
			return "", "", err
		}

		var title string

		if n.title != nil {
			title, _ = n.title(ctx, req, res)
		}

		if title == "" {
			title = "NextDHCP"
		}

		return title, msg, nil
	}

	return "", "", nil
}

func (n *notification) Send(title, msg string) error {
	gotifyURL, err := url.Parse(n.srv)
	if err != nil {
		return err
	}

	cli := gotify.NewClient(gotifyURL, &http.Client{})

	params := message.NewCreateMessageParams()
	params.Body = &models.MessageExternal{
		Title:    title,
		Message:  msg,
		Priority: 5,
	}

	_, err = cli.Message.CreateMessage(params, auth.TokenAuth(n.token))
	if err != nil {
		return err
	}

	return nil
}

// addNotification adds a new notification to the gotify plugin
func (g *gotifyPlugin) addNotification(n *notification) {
	g.notifications = append(g.notifications, n)
}

// findLastCreds returns the last credentials used for a notification
func (g *gotifyPlugin) findLastCreds() (string, string, bool) {
	if len(g.notifications) == 0 {
		return "", "", false
	}

	last := g.notifications[len(g.notifications)-1]
	return last.srv, last.token, true
}

// Name returns "gotify" and implements plugin.Handler
func (g *gotifyPlugin) Name() string {
	return "gotify"
}

// ServeDHCP checks if we should send a notification for that DHCP message
func (g *gotifyPlugin) ServeDHCP(ctx context.Context, req, res *dhcpv4.DHCPv4) error {
	// let the whole handler chain pass through
	if err := g.next.ServeDHCP(ctx, req, res); err != nil {
		return err
	}

	// kick of notifications in dedicated go routines
	for _, n := range g.notifications {
		go func(n *notification) {
			title, body, err := n.Prepare(ctx, req, res)
			if err != nil {
				g.l.Warnf("failed to pepare notification: %s", err.Error())
				return
			}

			if body != "" {
				g.l.Debugf("sending notification: %s\n%s", title, body)

				if err := n.Send(title, body); err != nil {
					g.l.Warnf("failed to send notification: %s", err.Error())
				} else {
					g.l.Debugf("notification sent via %s: %s\n%s", n.srv, title, body)
				}
			}
		}(n)
	}

	return nil
}
