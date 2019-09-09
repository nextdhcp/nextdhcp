// Package matcher provides a simple "rule" language that may be used
// inside NextDHCP plugin directives. The matcher library is based on
// github.com/Knetic/govaluate
package matcher

import (
	"fmt"
	"strings"

	"github.com/Knetic/govaluate"
	"github.com/caddyserver/caddy"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

type (
	// Matcher is a DHCP message matcher
	Matcher struct {
		// expr holds the pre-compiled expression
		expr *govaluate.EvaluableExpression
	}

	// ExprFunc can be used expose functions to matcher expressions
	ExprFunc func(args ...interface{}) (interface{}, error)
)

// SetupMatcher parses the current dispenser block and returns a DHCP
// message matcher
func SetupMatcher(c *caddy.Controller, fns ...map[string]ExprFunc) (*Matcher, error) {
	var conds []string
	var op = "&&"
	var disp = c.Dispenser // get a copy of the dispenser so we don't actually

	for disp.NextBlock() {
		switch disp.Val() {
		case "if":
			conds = append(conds, strings.Join(disp.RemainingArgs(), " "))
		case "if_op":
			if !disp.NextArg() {
				return nil, disp.ArgErr()
			}

			switch disp.Val() {
			case "and":
				fallthrough
			case "&&":
				op = "&&"
			case "or":
				fallthrough
			case "||":
				op = "||"
			default:
				return nil, c.ArgErr()
			}
		}
	}

	exprStr := ""

	for i, c := range conds {
		if i > 0 {
			exprStr += " " + op + " "
		}
		exprStr += "(" + c + ")"
	}

	functions := make(map[string]govaluate.ExpressionFunction)

	for _, m := range fns {
		for name, fn := range m {
			functions[name] = govaluate.ExpressionFunction(fn)
		}
	}

	var expr *govaluate.EvaluableExpression

	if exprStr != "" {
		var err error

		expr, err = govaluate.NewEvaluableExpressionWithFunctions(exprStr, functions)
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
func (m *Matcher) Match(request, response dhcpv4.DHCPv4) (bool, error) {
	if m.expr == nil {
		return true, nil
	}

	result, err := m.expr.Evaluate(map[string]interface{}{
		"request":  request,
		"response": response,
	})

	if err != nil {
		return false, err
	}
	if b, ok := result.(bool); ok {
		return b, nil
	}

	return false, fmt.Errorf("expression did not evaluate to a boolean. instead, got: %v", result)
}
