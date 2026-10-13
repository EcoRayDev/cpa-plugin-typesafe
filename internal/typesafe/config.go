package typesafe

import (
	"errors"

	"gopkg.in/yaml.v3"
)

type ConfigYAML struct {
	Model           string `yaml:"model"`
	CredentialsFile string `yaml:"credentials_file"`
}

func ParseConfig(data []byte) (Config, error) {
	var value ConfigYAML
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &value); err != nil {
			return Config{}, errors.New("invalid TypeSafe plugin config")
		}
	}
	return Config{Model: value.Model, CredentialsFile: value.CredentialsFile}, nil
}
