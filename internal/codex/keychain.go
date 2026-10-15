package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// codexKeychainService is the macOS Keychain service name codex-cli uses for
// its login. Source: openai/codex tag rust-v0.162.1 (last commit touching the
// file: a44afa527cf11114aed19ec893491bfc8d3931bb),
// codex-rs/login/src/auth/storage.rs, `const KEYRING_SERVICE`:
//
//	const KEYRING_SERVICE: &str = "Codex Auth";
const codexKeychainService = "Codex Auth"

// errKeychainCredentialsNotFound means the Keychain has no Codex login item.
var errKeychainCredentialsNotFound = errors.New("Codex Keychain credentials not found")

// errKeychainTimeout is returned when /usr/bin/security blocks, almost always
// on a Keychain permission dialog that SSH cannot show or confirm.
var errKeychainTimeout = errors.New("timed out reading Codex credentials from Keychain")

const keychainPromptHint = "a permission dialog may be waiting on the Mac desktop (SSH cannot confirm it). If you already ran `security unlock-keychain`, authorize /usr/bin/security in Keychain Access or run `codex login` on the desktop"

// keychainWhere names the Keychain store in error messages, the way loadAuth
// names the auth.json path.
const keychainWhere = `the macOS Keychain (service "Codex Auth")`

// These private seams let the keychain path be tested without touching a real
// Keychain. Production code always uses the platform implementations.
var readKeychainAuth = loadKeychainAuth
var readKeychainItem = loadKeychainItem
var updateKeychainItem = persistKeychainAuth

// storeKey derives the Keychain account name codex-cli uses for a CODEX_HOME,
// mirroring its compute_store_key: the SHA-256 of the canonicalized home, hex
// encoded, truncated to 16 characters, prefixed with "cli|". Same source as
// codexKeychainService above, `fn compute_store_key`:
//
//	fn compute_store_key(codex_home: &Path) -> std::io::Result<String> {
//	    let canonical = codex_home.canonicalize()...;
//	    hasher.update(path_str.as_bytes()); ...
//	    Ok(format!("cli|{truncated}"))
//	}
func storeKey(codexHome string) string {
	canonical := codexHome
	if abs, err := filepath.Abs(codexHome); err == nil {
		canonical = abs
	}
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = resolved
	}
	sum := sha256.Sum256([]byte(canonical))
	return "cli|" + hex.EncodeToString(sum[:])[:16]
}

func classifySecurityError(err error, stderr string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, errKeychainTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s", errKeychainTimeout, keychainPromptHint)
	}
	msg := strings.TrimSpace(stderr)
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 44 {
		return errKeychainCredentialsNotFound
	}
	if strings.Contains(msg, "could not be found") {
		return errKeychainCredentialsNotFound
	}
	if isKeychainInteractionError(msg) {
		return fmt.Errorf("Keychain is locked or a permission dialog cannot be shown. Run `security unlock-keychain` in this terminal, then retry. If it still fails: %s", keychainPromptHint)
	}
	if msg == "" {
		return fmt.Errorf("reading Codex credentials from Keychain: %w", err)
	}
	return fmt.Errorf("reading Codex credentials from Keychain: %s", msg)
}

func isKeychainInteractionError(msg string) bool {
	return strings.Contains(msg, "User interaction is not allowed") ||
		strings.Contains(msg, "could not be found in this keychain, because it is locked") ||
		strings.Contains(msg, "Unable to display a dialogue")
}
