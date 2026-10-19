package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCredentialsPrefersFileOverKeychain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"claudeAiOauth":{"accessToken":"file-token","refreshToken":"file-refresh"}}`)
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	original := readKeychainCredentials
	t.Cleanup(func() { readKeychainCredentials = original })
	readKeychainCredentials = func(context.Context) ([]byte, error) {
		return []byte(`{"accessToken":"keychain-token"}`), nil
	}

	o, err := loadCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o.AccessToken != "file-token" {
		t.Fatalf("AccessToken = %q, want file-token", o.AccessToken)
	}
}

func TestLoadCredentialsFallsBackToKeychainWhenFileHasNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	original := readKeychainCredentials
	t.Cleanup(func() { readKeychainCredentials = original })
	readKeychainCredentials = func(context.Context) ([]byte, error) {
		return []byte(`{"accessToken":"keychain-token"}`), nil
	}

	o, err := loadCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o.AccessToken != "keychain-token" {
		t.Fatalf("AccessToken = %q, want keychain-token", o.AccessToken)
	}
}

func TestLoadCredentialsFallsBackToKeychain(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	original := readKeychainCredentials
	t.Cleanup(func() { readKeychainCredentials = original })
	readKeychainCredentials = func(context.Context) ([]byte, error) {
		return []byte(`{"accessToken":"keychain-token"}`), nil
	}

	o, err := loadCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o.AccessToken != "keychain-token" {
		t.Fatalf("AccessToken = %q, want keychain-token", o.AccessToken)
	}
}
