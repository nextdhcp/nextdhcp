package storage

import (
	"errors"
)

// Factory creates a new LeaseStorage
type Factory func(args map[string][]string) (LeaseStorage, error)

var registeredFactorys = map[string]Factory{}

// Register registeres a new storage factory
func Register(name string, factory Factory) error {
	if _, ok := registeredFactorys[name]; ok {
		return errors.New("storage driver already registered")
	}

	registeredFactorys[name] = factory
	return nil
}

// MustRegister registeres a new storage factory and panics on error
func MustRegister(name string, factory Factory) {
	if err := Register(name, factory); err != nil {
		panic(err)
	}
}

// Open opens a lease.Database using driver name
func Open(name string, args map[string][]string) (LeaseStorage, error) {
	factory, ok := registeredFactorys[name]
	if !ok {
		return nil, errors.New("unknown driver")
	}

	return factory(args)
}
