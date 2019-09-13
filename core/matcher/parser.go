package matcher

import (
	"strings"

	"github.com/caddyserver/caddy"
)

// ParseConditions parses the current dispenser block for if and if_op conditions
// and returns them as a single, concatenated expression string usable for
// govaluate.NewEvaluableExpression() and similar
func ParseConditions(c *caddy.Controller) (string, error) {
	var conds []string
	var op = "&&"
	var disp = c.Dispenser // get a copy of the dispenser so we don't actually

	for disp.NextBlock() {
		switch disp.Val() {
		case "if":
			conds = append(conds, strings.Join(disp.RemainingArgs(), " "))
		case "if_op":
			if !disp.NextArg() {
				return "", disp.ArgErr()
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
				return "", c.ArgErr()
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

	return exprStr, nil
}
