package memory

import (
	"github.com/nextdhcp/nextdhcp/core/lease"
)

func factory(opts map[string][]string) (lease.Database, error) {
	return New(), nil
}

func init() {
	lease.MustRegisterDriver("memory", factory)
}
