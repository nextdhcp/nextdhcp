package gotify

import (
	"context"
	"testing"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
)

func assertNotification(t *testing.T, n *notification, msg, title, srv, token string) {
	assert.Equal(t, srv, n.srv)
	assert.Equal(t, token, n.token)

	ctx, _ := test.WithReplacer(context.Background())

	if n.msg != nil {
		m, err := n.msg(ctx, nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, msg, m)
	} else {
		assert.Equal(t, "", msg)
	}

	if n.title != nil {
		m, err := n.title(ctx, nil, nil)
		assert.NoError(t, err)
		assert.Equal(t, title, m)
	} else {
		assert.Equal(t, "", title)
	}
}

func TestGotifySetup(t *testing.T) {
	input := `
	gotify msgtype == 'DISCOVER' {
		message "Some cool message"
		title "with an even better title"
		server http://gotify.com some-app-token
	}
	`
	c := caddy.NewTestController("dhcpv4", input)
	g, err := makeGotifyPlugin(c)
	assert.NoError(t, err)
	assert.Len(t, g.notifications, 1)
	assert.NotNil(t, g.notifications[0].Matcher)
	assert.False(t, g.notifications[0].Matcher.EmptyCondition())
	assertNotification(t, g.notifications[0], "Some cool message", "with an even better title", "http://gotify.com", "some-app-token")

	input = `
	gotify {
		message	
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err)

	input = `
	gotify {
		title	
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err)

	input = `
	gotify {
		server	
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err)

	input = `
	gotify {
		server http://gotifiy.com
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err)

	input = `
	gotify {
		unknown-key	
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err)

	input = `
	gotify {
		server http://gotify.com some-app-token	
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.NoError(t, err)

	input = `
	gotify msgtype == 'REQUEST' {
		title some-title
	}`
	c = caddy.NewTestController("dhcpv4", input)
	_, err = makeGotifyPlugin(c)
	assert.Error(t, err) // msg must be set if condition is used

	// server and token configuration should propergate to notifications
	// defined below them
	input = `
	gotify {
		server http://gotify.com some-app-token
	}
	
	gotify {
		message "some message"
	}
	
	gotify {
		server http://example.com another-token
	}
	
	gotify {
		message "another message"
	}
	`
	c = caddy.NewTestController("dhcpv4", input)
	g, err = makeGotifyPlugin(c)
	assert.NoError(t, err)
	assert.Len(t, g.notifications, 4)
	assertNotification(t, g.notifications[1], "some message", "", "http://gotify.com", "some-app-token")
	assertNotification(t, g.notifications[3], "another message", "", "http://example.com", "another-token")
}
