package lease

import (
	"errors"
)

// Factory is a database factory function
type Factory func(opts map[string][]string) (Database, error)

var drivers = map[string]Factory{}

// RegisterDriver registers a new lease.Database driver factory
func RegisterDriver(name string, factory Factory) error {
	if _, ok := drivers[name]; ok {
		return errors.New("driver already registered")
	}

	drivers[name] = factory

	return nil
}

// MustRegisterDriver is like RegisterDriver but panics on error
func MustRegisterDriver(name string, factory Factory) {
	if err := RegisterDriver(name, factory); err != nil {
		panic(err)
	}
}

// Open opens a lease.Database using driver name
func Open(name string, args map[string][]string) (Database, error) {
	factory, ok := drivers[name]
	if !ok {
		return nil, errors.New("unknown driver")
	}

	return factory(args)
}

// MustOpen is like Open but panics on error
func MustOpen(name string, args map[string][]string) Database {
	db, err := Open(name, args)
	if err != nil {
		panic(err)
	}

	return db
}
