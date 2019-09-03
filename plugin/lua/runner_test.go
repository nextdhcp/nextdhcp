package lua

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewFromReader_success(t *testing.T) {
	runner, err := NewFromReader(bytes.NewBufferString(`
	subnet "10.1.1.1/24" {
		ranges = {
			{"10.1.1.100", "10.1.1.200"}
		}
	}
	
	subnet "10.1.2.1/8" {
		ranges = {}
	}
	
	plugin "foobar" {
		path = "path/to/foobar/so",
		something = "else"
	}
	`))

	assert.NoError(t, err)
	assert.NotNil(t, runner)
	assert.Len(t, runner.Subnets(), 2)
	assert.Len(t, runner.Plugins(), 1)
}

func Test_NewFromReader_sytax_error(t *testing.T) {
	runner, err := NewFromReader(bytes.NewBufferString(`
	{)
	`))

	assert.Error(t, err)
	assert.Nil(t, runner)
}

func Test_NewFromReader_runtime_error(t *testing.T) {
	runner, err := NewFromReader(bytes.NewBufferString(`
		nonExistingMethodCall()
	`))

	assert.Error(t, err)
	assert.Nil(t, runner)
}
