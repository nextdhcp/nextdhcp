// Package matcher provides a simple "rule" language that may be used
// inside NextDHCP plugin directives. The matcher library is based on
// github.com/Knetic/govaluate
package matcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knetic/govaluate"
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/nextdhcp/nextdhcp/core/replacer"
)

type (
	// Matcher is a DHCP message matcher
	Matcher struct {
		// expr holds the pre-compiled expression
		expr *govaluate.EvaluableExpression
	}

	// ExprFunc can be used expose functions to matcher expressions
	ExprFunc func(args ...interface{}) (interface{}, error)

	// evalParams implements the govaluate.Parameters interface and works on top of
	// a DHCPv4 message
	evalParams struct {
		replacer replacer.Replacer
		req      *dhcpv4.DHCPv4
	}
)

// SetupMatcher parses the current dispenser block and returns a DHCP
// message matcher
func SetupMatcher(c *caddy.Controller, fns ...map[string]ExprFunc) (*Matcher, error) {
	exprStr, err := ParseConditions(c)
	if err != nil {
		return nil, err
	}

	return SetupMatcherString(exprStr, fns...)
}

// SetupMatcherRemainingArgs creates a new DHCPv4 coniditon matcher from the remaining args
// available in the current dispenser line
func SetupMatcherRemainingArgs(c *caddy.Controller, fns ...map[string]ExprFunc) (*Matcher, error) {
	exprStr := strings.Join(c.RemainingArgs(), " ")

	return SetupMatcherString(exprStr, fns...)
}

// SetupMatcherString creates a new DHCPv4 condition matcher form the provided string
func SetupMatcherString(exprString string, fns ...map[string]ExprFunc) (*Matcher, error) {
	functions := make(map[string]govaluate.ExpressionFunction)

	for _, m := range fns {
		for name, fn := range m {
			functions[name] = govaluate.ExpressionFunction(fn)
		}
	}

	var expr *govaluate.EvaluableExpression
	if exprString != "" {
		var err error

		expr, err = govaluate.NewEvaluableExpressionWithFunctions(exprString, functions)
		if err != nil {
			return nil, err
		}
	}

	return &Matcher{
		expr: expr,
	}, nil
}

// Match evaluates the expression stored in the matcher against the given request and response
// message
func (m *Matcher) Match(ctx context.Context, request *dhcpv4.DHCPv4) (bool, error) {
	if m.expr == nil {
		return true, nil
	}

	params := prepareEvalContext(ctx, request)
	return m.MatchParams(params)
}

// EmptyCondition returns true if there's no condition for the matcher.
// In this case, any call to Match or MatchParams will return true
func (m *Matcher) EmptyCondition() bool {
	return m.expr == nil
}

// MatchParams executes the parsed govaluate expression and returns the resulting
// boolean output. If the return value of the expression is not a boolean an error
// is returned
func (m *Matcher) MatchParams(params govaluate.Parameters) (bool, error) {
	if m.expr == nil {
		return true, nil
	}

	result, err := m.expr.Eval(params)
	if err != nil {
		return false, err
	}
	if b, ok := result.(bool); ok {
		return b, nil
	}

	return false, fmt.Errorf("expression did not evaluate to a boolean. instead, got: %v", result)
}

// prepareEvalContext returns a new govaluate.Parameters interface that
// works by extracting data from request
func prepareEvalContext(ctx context.Context, request *dhcpv4.DHCPv4) govaluate.Parameters {
	rep := replacer.NewReplacer(ctx, request)
	return &evalParams{
		replacer: rep,
		req:      request,
	}
}

func (e *evalParams) Get(name string) (interface{}, error) {
	if e.replacer != nil {
		value := e.replacer.Get(name)
		return value, nil
	}

	return nil, fmt.Errorf("unknown key")
}

var _ govaluate.Parameters = &evalParams{}
