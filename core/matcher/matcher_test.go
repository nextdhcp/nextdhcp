package matcher

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/caddyserver/caddy"
	"github.com/caddyserver/caddy/caddyfile"
	"github.com/stretchr/testify/assert"
)

func TestSetupMatcher(t *testing.T) {
	testCases := []struct {
		input       string
		ifOp        string
		expectedErr bool
	}{
		{
			"msgtype == DISCOVER",
			"and",
			false,
		},
		{
			"",
			"or",
			false,
		},
		{
			"==",
			"",
			true,
		},
	}

	for i, c := range testCases {
		testCase := fmt.Sprintf("case #%d", i+1)
		input := fmt.Sprintf("{\nif %s\nif_op %s\n}", c.input, c.ifOp)
		disp := caddyfile.NewDispenser(testCase, bytes.NewBufferString(input))
		m, err := SetupMatcher(&caddy.Controller{Dispenser: disp})
		if c.expectedErr {
			assert.Error(t, err, testCase)
			assert.Nil(t, m, testCase)
		} else {
			assert.NoError(t, err, testCase)
			assert.NotNil(t, m, testCase)
		}

	}
}
