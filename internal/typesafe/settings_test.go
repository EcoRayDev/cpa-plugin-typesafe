package typesafe

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsBlankKeyPreservesSavedCredentialAndReloads(t *testing.T) {
	store := filepath.Join(t.TempDir(), "private", "credential.yaml")
	p, err := NewWithStore(Config{Model: "jev-custom", CredentialsFile: store, APIKey: "bootstrap-secret"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.SaveSettings(context.Background(), Settings{Model: "jev-new", APIKey: "replacement-secret"}); err != nil {
		t.Fatal(err)
	}
	if err = p.SaveSettings(context.Background(), Settings{Model: "jev-final"}); err != nil {
		t.Fatal(err)
	}
	if p.APIKey() != "replacement-secret" || p.Model() != "jev-final" {
		t.Fatal("runtime settings not retained")
	}
	reloaded, err := NewWithStore(Config{CredentialsFile: store}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.APIKey() != "replacement-secret" || reloaded.Model() != "jev-final" {
		t.Fatal("saved settings did not reload")
	}
	info, err := os.Stat(store)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("file mode=%o", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(store))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0700 {
		t.Fatalf("dir mode=%o", dir.Mode().Perm())
	}
}
func TestSavedCredentialOverridesEnvironmentBootstrap(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "environment-bootstrap")
	path := filepath.Join(privateTestDir(t), "credential.yaml")
	if err := os.WriteFile(path, []byte("api_key: saved-key\nmodel: jev-latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := NewWithStore(Config{CredentialsFile: path}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if p.APIKey() != "saved-key" {
		t.Fatal("environment key overrode persisted key")
	}
}
func TestCredentialFileSymlinkAndBroadPermissionsRejected(t *testing.T) {
	dir := privateTestDir(t)
	outside := filepath.Join(dir, "outside.yaml")
	if err := os.WriteFile(outside, []byte("api_key: untouched\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	p, err := NewWithStore(Config{CredentialsFile: link}, http.DefaultClient)
	if err == nil || p != nil {
		t.Fatal("symlink credential file accepted")
	}
	broad := filepath.Join(dir, "broad.yaml")
	if err := os.WriteFile(broad, []byte("api_key: leaked\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = NewWithStore(Config{CredentialsFile: broad}, http.DefaultClient); err == nil {
		t.Fatal("group-readable file accepted")
	}
}
