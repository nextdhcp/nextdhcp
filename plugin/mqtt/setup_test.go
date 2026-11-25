package mqtt

import (
	"context"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/nextdhcp/nextdhcp/plugin/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeMqttPlugin sets up the plugin from input and provisions it, like a
// server does when it loads the plugin from its JSON config.
func makeMqttPlugin(t *testing.T, input string) (*mqttPlugin, error) {
	m := new(mqttPlugin)
	if err := m.UnmarshalCaddyfile(caddyfile.NewTestDispenser(input)); err != nil {
		return nil, err
	}

	mod, err := test.Provision(t, m)
	if err != nil {
		return nil, err
	}

	return mod.(*mqttPlugin), nil
}

func TestMqttSetup(t *testing.T) {
	m, err := makeMqttPlugin(t, `
	mqtt {
		name local
		broker tcp://127.0.0.1:1883 tcp://127.0.0.2:1883
		user someone
		password secret
		clean-session
		qos 1
		topic some/topic
		payload "some payload"
	}

	mqtt msgtype == 'REQUEST' {
		use local
		topic other/topic
		payload-from echo hello
	}`)
	require.NoError(t, err)
	require.Len(t, m.configs, 2)

	ctx, _ := test.WithReplacer(context.Background())

	first := m.configs[0]
	assert.Equal(t, "local", first.name)
	assert.True(t, first.EmptyCondition())
	require.NotNil(t, first.conn)
	assert.Equal(t, []string{"tcp://127.0.0.1:1883", "tcp://127.0.0.2:1883"}, first.conn.broker)
	assert.Equal(t, "someone", first.conn.user)
	assert.Equal(t, "secret", first.conn.password)
	assert.True(t, first.conn.cleanSession)
	assert.Equal(t, 1, first.conn.qos)

	topic, err := first.topic(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "some/topic", topic)

	payload, err := first.payload(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "some payload", payload)

	// the second message uses the connection of the first one
	second := m.configs[1]
	assert.Equal(t, "local", second.name)
	assert.False(t, second.EmptyCondition())
	assert.Nil(t, second.conn)

	topic, err = second.topic(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "other/topic", topic)

	payload, err = second.payload(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", payload)
}

func TestMqttSetupPayloadOrder(t *testing.T) {
	// the payload option that comes last is used
	m, err := makeMqttPlugin(t, `
	mqtt {
		broker tcp://127.0.0.1:1883
		topic some/topic
		payload-from echo hello
		payload "some payload"
	}`)
	require.NoError(t, err)
	require.Len(t, m.configs, 1)

	ctx, _ := test.WithReplacer(context.Background())
	payload, err := m.configs[0].payload(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "some payload", payload)
}

func TestMqttSetupErrors(t *testing.T) {
	for _, input := range []string{
		// neither a connection nor one to use
		"mqtt {\n topic some/topic\n}",
		// both a connection and one to use
		"mqtt {\n broker tcp://127.0.0.1:1883\n use other\n}",
		"mqtt {\n use other\n broker tcp://127.0.0.1:1883\n}",
		"mqtt {\n broker tcp://127.0.0.1:1883\n qos 3\n}",
		"mqtt {\n broker\n}",
		"mqtt {\n use\n}",
		"mqtt {\n use other\n topic\n}",
		"mqtt {\n use other\n payload-from\n}",
		"mqtt == {\n use other\n}",
		// without a topic, a block can only configure a named connection
		"mqtt {\n broker tcp://127.0.0.1:1883\n}",
		"mqtt {\n use other\n}",
		"mqtt {\n use other\n payload foo\n}",
		"mqtt msgtype == 'DISCOVER' {\n name local\n broker tcp://127.0.0.1:1883\n}",
		"mqtt {\n name local\n broker tcp://127.0.0.1:1883\n payload foo\n}",
		"mqtt {\n name local\n broker tcp://127.0.0.1:1883\n payload-from echo foo\n}",
	} {
		_, err := makeMqttPlugin(t, input)
		assert.Error(t, err, input)
	}
}

func TestMqttSetupWithoutTopicOrPayload(t *testing.T) {
	m, err := makeMqttPlugin(t, `
	mqtt {
		name local
		broker tcp://127.0.0.1:1883
	}

	mqtt {
		use local
		topic some/topic
	}`)
	require.NoError(t, err)
	require.Len(t, m.configs, 2)

	// the first block only configures the connection
	assert.Nil(t, m.configs[0].topic)

	// the second one publishes an empty payload
	ctx, _ := test.WithReplacer(context.Background())
	require.NotNil(t, m.configs[1].payload)
	payload, err := m.configs[1].payload(ctx, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "", payload)
}

// TestMqttProvisionValidates checks that JSON configs are checked like
// Dhcpfile ones.
func TestMqttProvisionValidates(t *testing.T) {
	topic := "some/topic"

	for name, pub := range map[string]PublishConfig{
		"no connection":           {Topic: &topic},
		"no topic":                {Name: "local", Connection: &ConnectionConfig{}, Condition: "msgtype == 'DISCOVER'"},
		"unnamed connection only": {Connection: &ConnectionConfig{}},
	} {
		_, err := test.Provision(t, &mqttPlugin{Publish: []PublishConfig{pub}})
		assert.Error(t, err, name)
	}
}
