package gotify

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/apex/log"
	"github.com/gotify/go-api-client/v2/client/message"
	"github.com/gotify/go-api-client/v2/models"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/matcher"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationPrepare(t *testing.T) {
	ctx := context.Background()
	req, _ := dhcpv4.NewDiscovery(net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	res, _ := dhcpv4.NewReplyFromRequest(req)
	emptyMatcher, err := matcher.SetupMatcherString("")

	require.NoError(t, err)

	var (
		msg      string
		title    string
		msgErr   error
		titleErr error
	)

	n := notification{
		Matcher: emptyMatcher,
		msg: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
			return msg, msgErr
		},
		title: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
			return title, titleErr
		},
		srv:   "http://gotify.com",
		token: "some-token",
	}

	// empty tilte should be replaced with NextDHCP
	nt, nm, err := n.Prepare(ctx, req, res)
	assert.NoError(t, err)
	assert.Equal(t, "NextDHCP", nt)
	assert.Empty(t, nm)

	// msg and title should be set correclty
	msg = "some message"
	title = "some title"
	nt, nm, err = n.Prepare(ctx, req, res)
	assert.NoError(t, err)
	assert.Equal(t, "some title", nt)
	assert.Equal(t, "some message", nm)

	// should return empty strings if not matched
	alwaysFalse, err := matcher.SetupMatcherString("1 == 0")
	require.NoError(t, err)
	n.Matcher = alwaysFalse
	nt, nm, err = n.Prepare(ctx, req, res)
	assert.NoError(t, err)
	assert.Empty(t, nm)
	assert.Empty(t, nt)

	errorMacher, err := matcher.SetupMatcherString("'string'")
	require.NoError(t, err)
	n.Matcher = errorMacher
	nt, nm, err = n.Prepare(ctx, req, res)
	assert.Error(t, err)

	n.Matcher = emptyMatcher
	msgErr = errors.New("simulated error")
	nt, nm, err = n.Prepare(ctx, req, res)
	assert.Equal(t, msgErr, err)
	assert.Empty(t, nt)
	assert.Empty(t, nm)
}

func TestNotificatonSend(t *testing.T) {
	emptyMatcher, err := matcher.SetupMatcherString("")
	require.NoError(t, err)

	n := notification{
		Matcher: emptyMatcher,
		srv:     "http://gotify.com",
		token:   "some-token",
	}

	called := false
	returnErr := errors.New("simulated error")
	notify = func(srv *url.URL, token string, msg *message.CreateMessageParams) error {
		called = true

		assert.Equal(t, "http://gotify.com", srv.String())
		assert.Equal(t, "some-token", token)
		assert.Equal(t, "title", msg.Body.Title)
		assert.Equal(t, "message", msg.Body.Message)
		assert.Equal(t, 5, msg.Body.Priority)

		return returnErr
	}

	assert.Equal(t, returnErr, n.Send("title", "message"))
	assert.True(t, called)

}

func TestGotifyServeDHCP(t *testing.T) {
	emptyMatcher, _ := matcher.SetupMatcherString("")
	alwaysFalse, _ := matcher.SetupMatcherString("1 == 0")
	errorMacher, _ := matcher.SetupMatcherString("'string'")

	called := 0
	var messages []*models.MessageExternal

	notify = func(srv *url.URL, token string, msg *message.CreateMessageParams) error {
		called++

		messages = append(messages, msg.Body)

		return nil
	}

	g := &gotifyPlugin{
		notifications: []*notification{
			&notification{
				Matcher: emptyMatcher,
				msg: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "message1", nil
				},
				title: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "title1", nil
				},
				srv:   "http://gotify1.com",
				token: "some-token-1",
			},
			&notification{
				Matcher: alwaysFalse,
				msg: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "message2", nil
				},
				title: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "title2", nil
				},
				srv:   "http://gotify2.com",
				token: "some-token-2",
			},
			&notification{
				Matcher: errorMacher,
				msg: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "message3", nil
				},
				title: func(_ context.Context, req, res *dhcpv4.DHCPv4) (string, error) {
					return "title3", nil
				},
				srv:   "http://gotify3.com",
				token: "some-token-3",
			},
			&notification{
				Matcher: emptyMatcher,
				srv:     "http://gotify4.com",
				token:   "some-token-4",
			},
		},
		l:    log.Log,
		next: test.ErrorHandler,
	}

	req, _ := dhcpv4.NewDiscovery(net.HardwareAddr{0, 1, 2, 3, 4, 5})
	res, _ := dhcpv4.NewReplyFromRequest(req)
	assert.Error(t, g.ServeDHCP(context.Background(), req, res))
	g.wg.Wait()

	assert.Equal(t, 0, called)
	assert.Empty(t, messages)

	g.next = test.NoOpHandler
	assert.NoError(t, g.ServeDHCP(context.Background(), req, res))
	g.wg.Wait()

	assert.Equal(t, 1, called)
	assert.Equal(t, "message1", messages[0].Message)
}
