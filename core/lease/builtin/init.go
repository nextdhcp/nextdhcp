package builtin

import "github.com/nextdhcp/nextdhcp/core/lease"

func factory(opts map[string]interface{}) (lease.Database, error) {
	return New(), nil
}

func init() {
	lease.MustRegisterDriver("", factory)
}