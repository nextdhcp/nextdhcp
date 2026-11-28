package events

import (
	"runtime/debug"
	"sync"

	"github.com/nextdhcp/nextdhcp/core/lease"
	"github.com/rs/zerolog/log"
)

// EventName is the name of an event
type EventName string

// EventHook is a function that is called for every event that is emitted
type EventHook func(eventType EventName, eventInfo interface{}) error

const (
	// EventLeaseExpired is emitted when a client address lease expired
	EventLeaseExpired EventName = "lease-expired"

	// EventLeaseCreated is emitted when an address has been bound
	// to a client
	EventLeaseCreated = "lease-created"
)

type (
	// LeaseEventHook is the function type that can receive lease-based events
	LeaseEventHook func(event EventName, l *lease.Lease) error
)

var validLeaseEvents = map[EventName]struct{}{
	EventLeaseCreated: {},
	EventLeaseExpired: {},
}

// registeredHooks holds the registered event hooks by name
var registeredHooks = &sync.Map{}

// RegisterEventHook registers a hook that is called for every event that is
// emitted. Hooks must have a unique name; RegisterEventHook panics if the name
// is empty or already registered.
func RegisterEventHook(name string, hook EventHook) {
	if name == "" {
		panic("event hook must have a name")
	}

	_, dup := registeredHooks.LoadOrStore(name, hook)
	if dup {
		panic("hook named " + name + " already registered")
	}
}

// EmitEvent calls all registered event hooks with the event. It blocks until
// all hooks returned; errors returned by hooks are logged.
func EmitEvent(event EventName, info interface{}) {
	registeredHooks.Range(func(k, v interface{}) bool {
		err := v.(EventHook)(event, info)
		if err != nil {
			log.Error().Msgf("error on '%s' hook: %v", k.(string), err)
		}
		return true
	})
}

// EmitLeaseEvent emits a lease-based event
func EmitLeaseEvent(event EventName, l *lease.Lease) {
	if _, ok := validLeaseEvents[event]; !ok {
		log.Error().Msg("invalid lease event type")
		log.Error().Msg(string(debug.Stack()))
		return
	}

	EmitEvent(event, l)
}

// RegisterLeaseEventHook registers a new lease event hook
func RegisterLeaseEventHook(name string, event EventName, hook LeaseEventHook) {
	if _, ok := validLeaseEvents[event]; !ok {
		panic("invalid lease event name")
	}

	RegisterEventHook(name, func(e EventName, value interface{}) error {
		if event != e {
			return nil
		}

		return hook(event, value.(*lease.Lease))
	})
}
