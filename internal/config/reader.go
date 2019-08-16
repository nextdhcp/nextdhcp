package config

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/ghodss/yaml"
)

// ReadJSON reads a JSON encoded configuration from r
func ReadJSON(r io.Reader) (*Config, error) {
	var (
		cfg     Config
		decoder = json.NewDecoder(r)
	)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	if err := cfg.Prepare(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ReadYAML reads a YAML encoded configuration from r
func ReadYAML(r io.Reader) (*Config, error) {
	content, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	jsonBlob, err := yaml.YAMLToJSON(content)
	if err != nil {
		return nil, err
	}

	return ReadJSON(bytes.NewBuffer(jsonBlob))
}
