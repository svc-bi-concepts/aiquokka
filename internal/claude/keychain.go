package claude

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const claudeCredentialsService = "Claude Code-credentials"

// errKeychainTimeout is returned when /usr/bin/security blocks, almost always
// on a Keychain permission dialog that SSH cannot show or confirm.
var errKeychainTimeout = errors.New("timed out reading Claude Code credentials from Keychain")

const keychainPromptHint = "a permission dialog may be waiting on the Mac desktop (SSH cannot confirm it). If you already ran `security unlock-keychain`, authorize /usr/bin/security in Keychain Access or copy the OAuth JSON into ~/.claude/.credentials.json"

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
		return fmt.Errorf("reading Claude Code credentials from Keychain: %w", err)
	}
	return fmt.Errorf("reading Claude Code credentials from Keychain: %s", msg)
}

func isKeychainInteractionError(msg string) bool {
	return strings.Contains(msg, "User interaction is not allowed") ||
		strings.Contains(msg, "could not be found in this keychain, because it is locked") ||
		strings.Contains(msg, "Unable to display a dialogue")
}

// decodeSecurityHex undoes the hex encoding /usr/bin/security falls back to
// for `find-generic-password -w` output that is not plain text: when the
// stored bytes hold newlines or other non-printable characters, security
// prints them as lowercase hex instead. Pretty-printed JSON, which Claude
// Code now writes, trips that path. When the input looks like such a hex
// dump and decodes to JSON that starts with '{', the decoded bytes are
// returned; anything else is returned unchanged. The decoded bytes are
// secret and are never logged.
func decodeSecurityHex(stdout []byte) []byte {
	if len(stdout) == 0 || len(stdout)%2 != 0 {
		return stdout
	}
	for _, c := range stdout {
		if !isLowerHexDigit(c) {
			return stdout
		}
	}
	decoded := make([]byte, len(stdout)/2)
	if _, err := hex.Decode(decoded, stdout); err != nil {
		return stdout
	}
	if decoded[0] != '{' {
		return stdout
	}
	return decoded
}

func isLowerHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f'
}
