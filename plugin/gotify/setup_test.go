package gotify

import (
	"context"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
)

// makeGotifyPlugin sets up the plugin from input and provisions it, like a
// server does when it loads the plugin from its JSON config.
func makeGotifyPlugin(t *testing.T, input string) (*gotifyPlugin, error) {
	g := new(gotifyPlugin)
	if err := g.UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)); err != nil {
		return nil, err
	}

	m, err := test.Provision(t, g)
	if err != nil {
		return nil, err
	}

	return m.(*gotifyPlugin), nil
}

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
	g, err := makeGotifyPlugin(t, input)
	assert.NoError(t, err)
	assert.Len(t, g.notifications, 1)
	assert.NotNil(t, g.notifications[0].Matcher)
	assert.False(t, g.notifications[0].Matcher.EmptyCondition())
	assertNotification(t, g.notifications[0], "Some cool message", "with an even better title", "http://gotify.com", "some-app-token")

	input = `
	gotify {
		message	
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.Error(t, err)

	input = `
	gotify {
		title	
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.Error(t, err)

	input = `
	gotify {
		server	
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.Error(t, err)

	input = `
	gotify {
		server http://gotifiy.com
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.Error(t, err)

	input = `
	gotify {
		unknown-key	
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.Error(t, err)

	input = `
	gotify {
		server http://gotify.com some-app-token	
	}`
	_, err = makeGotifyPlugin(t, input)
	assert.NoError(t, err)

	input = `
	gotify msgtype == 'REQUEST' {
		title some-title
	}`
	_, err = makeGotifyPlugin(t, input)
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
	g, err = makeGotifyPlugin(t, input)
	assert.NoError(t, err)
	assert.Len(t, g.notifications, 4)
	assertNotification(t, g.notifications[1], "some message", "", "http://gotify.com", "some-app-token")
	assertNotification(t, g.notifications[3], "another message", "", "http://example.com", "another-token")
}
