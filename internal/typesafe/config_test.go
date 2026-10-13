package typesafe

import (
	"strings"
	"testing"
)

func TestConfigOnlyAcceptsSecretFreeRuntimeFields(t *testing.T) {
	config, err := ParseConfig([]byte("model: jev-latest\ncredentials_file: /tmp/typesafe-credentials.yaml\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Model != "jev-latest" || config.CredentialsFile != "/tmp/typesafe-credentials.yaml" || config.APIKey != "" {
		t.Fatalf("config=%#v", config)
	}
	if strings.Contains(string((&Plugin{model: config.Model, credentialPath: config.CredentialsFile}).ConfigYAML()), "api_key") {
		t.Fatal("API key appeared in plugin host config YAML")
	}
}
