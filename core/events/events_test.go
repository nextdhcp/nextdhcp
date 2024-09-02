package events

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var eventHooks = &sync.Map{}

func init() {
	caddy.RegisterEventHook("event-testing-hook", func(name caddy.EventName, info interface{}) error {
		eventHooks.Range(func(_, value interface{}) bool {
			err := value.(caddy.EventHook)(name, info)
			if err != nil {
				return false
			}
			return false
		})

		return nil
	})
}

func registerTestingHook(name string, hook caddy.EventHook) {
	eventHooks.LoadOrStore(name, hook)
}

func removeTestingHook(name string) {
	eventHooks.Delete(name)
}

func TestEmitLeaseEvent(t *testing.T) {
	l := lease.Lease{
		Address: net.IP{192, 168, 0, 1},
		Expires: time.Now().Add(time.Minute),
		Client: lease.Client{
			Hostname: "client",
			HwAddr:   net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			ID:       "client",
		},
	}

	firedLeaseCreated := false
	registerTestingHook("test-emit-lease-event", func(name caddy.EventName, info interface{}) error {
		firedLeaseCreated = true
		assert.Equal(t, EventLeaseCreated, string(name))

		lp, ok := info.(*lease.Lease)
		require.True(t, ok)
		assert.Equal(t, &l, lp)

		return nil
	})
	defer removeTestingHook("test-emit-lease-event")

	EmitLeaseEvent(EventLeaseCreated, &l)
	assert.True(t, firedLeaseCreated)
}

func TestEmitLeaseEvent_invalid_name(t *testing.T) {
	eventFired := false
	registerTestingHook("test-emit-lease-event-invalid-name", func(name caddy.EventName, info interface{}) error {
		eventFired = true
		return nil
	})
	defer removeTestingHook("test-emit-lease-event-invalid-name")

	EmitLeaseEvent("invalid-name", nil)

	assert.False(t, eventFired)
}

func TestRegisterLeaseEventHook(t *testing.T) {
	called := false
	RegisterLeaseEventHook("test-register-hook", EventLeaseCreated, func(e caddy.EventName, l *lease.Lease) error {
		called = true
		return nil
	})

	caddy.EmitEvent("some-other-event", nil)
	assert.False(t, called, "should have been filtered")

	caddy.EmitEvent(EventLeaseExpired, &lease.Lease{})
	assert.False(t, called, "should have been filtered")

	caddy.EmitEvent(EventLeaseCreated, &lease.Lease{})
	assert.True(t, called, "should have been emitted")
}

func TestRegisterLeaseEventHook_panic(t *testing.T) {
	assert.Panics(t, func() {
		RegisterLeaseEventHook("should-panic-hook", "invalid-event-type", nil)
	})
}
