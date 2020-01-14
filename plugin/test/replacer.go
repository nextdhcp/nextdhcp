package test

import (
	"context"

	"github.com/nextdhcp/nextdhcp/core/replacer"
)

type (
	// SetFunc is called when Replacer.Set is called
	SetFunc func(string, replacer.Value)

	// GetFunc is called when Replacer.Get is called
	GetFunc func(string) string

	// ReplaceFunc is called when Replacer.Replace is called
	ReplaceFunc func(string) string

	// Replacer implements the replacer.Replacer interface
	Replacer struct {
		Getter   GetFunc
		Setter   SetFunc
		Replacer ReplaceFunc
	}
)

// Get implements replacer.Replacer
func (r *Replacer) Get(key string) string {
	if r.Getter != nil {
		return r.Getter(key)
	}

	return key
}

// Set implemes replacer.Replacer
func (r *Replacer) Set(key string, v replacer.Value) {
	if r.Setter != nil {
		r.Setter(key, v)
	}
}

// Replace implements replacer.Replacer
func (r *Replacer) Replace(input string) string {
	if r.Replacer != nil {
		return r.Replacer(input)
	}

	return input
}

// WithReplacer returns a context that has a test replacer assigned
func WithReplacer(ctx context.Context) (context.Context, *Replacer) {
	r := &Replacer{}
	return replacer.WithReplacer(ctx, r), r
}
