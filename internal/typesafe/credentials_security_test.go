package typesafe

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialStoreRejectsUnsafeDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "credential.yaml")
	if err := os.Mkdir(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("api_key: must-not-load\n"), 0600); err != nil {
		t.Fatal(err)
	}

	plugin, err := NewWithStore(Config{CredentialsFile: path}, http.DefaultClient)
	if err == nil || plugin != nil {
		t.Fatal("credential in an unsafe directory was accepted")
	}
	if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "must-not-load") {
		t.Fatal("credential error disclosed a path or credential")
	}
}

func TestCredentialStoreRejectsDirectorySymlinks(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "credential.yaml")
	if err := os.WriteFile(outsideFile, []byte("api_key: outside-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(base, "private")
	if err := os.Symlink(outside, linkedDir); err != nil {
		t.Fatal(err)
	}

	plugin, err := NewWithStore(Config{CredentialsFile: filepath.Join(linkedDir, "credential.yaml")}, http.DefaultClient)
	if err == nil || plugin != nil {
		t.Fatal("credential beneath a directory symlink was accepted")
	}
	if got, readErr := os.ReadFile(outsideFile); readErr != nil || string(got) != "api_key: outside-secret\n" {
		t.Fatalf("outside credential changed: err=%v", readErr)
	}
}

func TestCredentialStoreRejectsNonRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.yaml")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	plugin, err := NewWithStore(Config{CredentialsFile: path}, http.DefaultClient)
	if err == nil || plugin != nil {
		t.Fatal("directory accepted as credential file")
	}
}

func TestFailedCredentialReplacementPreservesExistingContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "credential.yaml")
	plugin, err := NewWithStore(Config{CredentialsFile: path, APIKey: "old-secret"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err = plugin.SaveSettings(context.Background(), Settings{Model: "jev-before", APIKey: "old-secret"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err = plugin.SaveSettings(context.Background(), Settings{Model: "jev-after", APIKey: "new-secret"}); err == nil {
		t.Fatal("replacement of an unsafe credential file succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := os.Stat(path)
	if string(after) != string(before) || statErr != nil || info.Mode().Perm() != 0644 {
		t.Fatal("failed replacement changed existing credential contents or mode")
	}
}

func TestNewCredentialStoreCreatesOwnerOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "credential.yaml")
	plugin, err := NewWithStore(Config{CredentialsFile: path, APIKey: "new-secret"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if err = plugin.SaveSettings(context.Background(), Settings{Model: "jev-latest"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("new credential file mode=%#o, regular=%t", info.Mode().Perm(), info.Mode().IsRegular())
	}
}
func TestCredentialStoreRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.yaml")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if _, err = file.Write(make([]byte, maxCredentialFileSize+1)); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = NewWithStore(Config{CredentialsFile: path}, http.DefaultClient); err == nil {
		t.Fatal("oversized credential file was accepted")
	}
}

func TestCredentialReplacementDoesNotFollowFileSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.yaml")
	if err := os.WriteFile(target, []byte("api_key: untouched\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "credential.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	plugin, err := NewWithStore(Config{CredentialsFile: filepath.Join(dir, "private", "credential.yaml"), APIKey: "bootstrap"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	plugin.credentialPath = link
	if err = plugin.SaveSettings(context.Background(), Settings{Model: "jev-new", APIKey: "replacement"}); err == nil {
		t.Fatal("replacement through a credential symlink succeeded")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "api_key: untouched\n" {
		t.Fatalf("symlink target changed: err=%v", err)
	}
}

func TestCredentialStoreRejectsOversizedReplacementWithoutChangingOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "credential.yaml")
	if err := writeCredentialFile(path, credentialFile{APIKey: "old-secret", Model: "jev-before"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeCredentialFile(path, credentialFile{APIKey: strings.Repeat("x", maxCredentialFileSize), Model: "jev-after"}); err == nil {
		t.Fatal("oversized credential replacement succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("oversized replacement changed existing credential: err=%v", err)
	}
}

func TestMissingCredentialPathPreservesNotExistError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "credential.yaml")
	if _, err := readCredentialFile(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing credential error=%v, want os.ErrNotExist", err)
	}
}
