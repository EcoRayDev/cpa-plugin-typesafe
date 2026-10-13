package main

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRegistrationRejectsUnsafeSchema(t *testing.T) {
	request, err := json.Marshal(lifecycleRequest{SchemaVersion: 6, ConfigYAML: []byte("model: jev-latest\n")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = register(request)
	if !errors.Is(err, errHostSchemaTooOld) {
		t.Fatalf("registration error=%v", err)
	}
}
