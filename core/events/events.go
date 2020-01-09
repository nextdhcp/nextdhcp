package events

import (
	"log"
	"runtime/debug"

	"github.com/caddyserver/caddy"
	"github.com/nextdhcp/nextdhcp/core/lease"
)

const (
	// EventLeaseExpired is emitted when a client address lease expired
	EventLeaseExpired caddy.EventName = "lease-expired"

	// EventLeaseCreated is emitted when an address has been bound
	// to a client
	EventLeaseCreated = "lease-created"
)

type (
	// LeaseEventHook is the function type that can receive lease-based events
	LeaseEventHook func(event caddy.EventName, l *lease.Lease) error
)

var (
	validLeaseEvents = map[caddy.EventName]struct{}{
		EventLeaseCreated: {},
		EventLeaseExpired: {},
	}
)

// EmitLeaseEvent emits a lease-based event
func EmitLeaseEvent(event caddy.EventName, l *lease.Lease) {
	if _, ok := validLeaseEvents[event]; !ok {
		log.Println("invalid lease event type")
		log.Println(debug.Stack())
		return
	}

	caddy.EmitEvent(event, l)
}

// RegisterLeaseEventHook registers a new lease event hook
func RegisterLeaseEventHook(event caddy.EventName, hook LeaseEventHook) {
	if _, ok := validLeaseEvents[event]; !ok {
		panic("invalid lease event name")
	}

	caddy.RegisterEventHook(string(event), func(e caddy.EventName, value interface{}) error {
		return hook(event, value.(*lease.Lease))
	})
}
