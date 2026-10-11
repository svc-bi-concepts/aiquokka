package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// codexKeychainService is the Keychain service codex-cli uses for its login
// when cli_auth_credentials_store = "keyring". Its account is "cli|" plus the
// first 16 hex digits of the SHA-256 of the canonical CODEX_HOME path.
const codexKeychainService = "Codex Auth"

var errKeychainNotFound = errors.New("Codex Keychain credentials not found")

// errKeychainTimeout is returned when /usr/bin/security blocks, almost always
// on a Keychain permission dialog that SSH cannot show or confirm.
var errKeychainTimeout = errors.New("timed out reading Codex credentials from Keychain: a permission dialog may be waiting on the Mac desktop")

// These seams let the keyring path be tested without touching a real
// Keychain. Production code always uses the platform implementations.
var readKeychain = loadKeychainItem
var writeKeychain = storeKeychainItem

// keychainAccount is the account codex-cli files its login under for home.
func keychainAccount(home string) (string, error) {
	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return "cli|" + hex.EncodeToString(sum[:])[:16], nil
}

// classifySecurityError maps a failed /usr/bin/security call to a Keychain
// sentinel when the item is absent, and otherwise to a readable error that
// never includes the secret.
func classifySecurityError(err error, stderr string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errKeychainTimeout) {
		return err
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 44 {
		return errKeychainNotFound
	}
	msg := strings.TrimSpace(stderr)
	if strings.Contains(msg, "could not be found") {
		return errKeychainNotFound
	}
	if msg == "" {
		return fmt.Errorf("reading Codex credentials from Keychain: %w", err)
	}
	return fmt.Errorf("reading Codex credentials from Keychain: %s", msg)
}
