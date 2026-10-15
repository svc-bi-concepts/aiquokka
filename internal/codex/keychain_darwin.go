//go:build darwin

package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// securityReadTimeout is how long we wait for /usr/bin/security before
// treating the call as blocked on a GUI dialog.
const securityReadTimeout = 3 * time.Second

// keychainMention extends the not-configured message to name both places a
// login can live.
const keychainMention = ` or in the macOS Keychain (service "Codex Auth")`

// loadKeychainAuth reads codex-cli's login from the macOS Keychain and returns
// the stored JSON (the same shape as auth.json) plus the account of the item,
// so a refreshed token is written back to the same item.
func loadKeychainAuth(ctx context.Context) ([]byte, []byte, error) {
	home, err := codexHome()
	if err != nil {
		return nil, nil, err
	}
	account := storeKey(home)
	data, err := securityFindPassword(ctx, codexKeychainService, account)
	if err != nil {
		return nil, nil, err
	}
	return data, []byte(account), nil
}

// loadKeychainItem re-reads one Keychain item by account for the refresh path.
func loadKeychainItem(ctx context.Context, item []byte) ([]byte, error) {
	return securityFindPassword(ctx, codexKeychainService, string(item))
}

// persistKeychainAuth overwrites the item the login was read from, the way
// codex-cli updates it.
func persistKeychainAuth(ctx context.Context, item, data []byte) error {
	account := string(item)
	if account == "" {
		return fmt.Errorf("updating Codex credentials in Keychain: missing account")
	}
	return securityAddPassword(ctx, codexKeychainService, account, data)
}

// securityFindPassword reads one generic password. Calls are time-bounded so a
// dialog over SSH cannot hang the process.
func securityFindPassword(ctx context.Context, service, account string) ([]byte, error) {
	args := []string{"find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	args = append(args, "-w")
	stdout, stderr, err := runSecurity(ctx, args...)
	if err != nil {
		return nil, classifySecurityError(err, string(stderr))
	}
	stdout = bytes.TrimSpace(stdout)
	if len(stdout) == 0 {
		return nil, errKeychainCredentialsNotFound
	}
	return stdout, nil
}

// securityAddPassword adds or updates the generic password, leaving the item's
// ACL as security leaves it.
func securityAddPassword(ctx context.Context, service, account string, data []byte) error {
	_, stderr, err := runSecurity(ctx,
		"add-generic-password",
		"-U",
		"-s", service,
		"-a", account,
		"-w", string(data),
	)
	if err != nil {
		if errors.Is(err, errKeychainTimeout) {
			return fmt.Errorf("updating Codex credentials in Keychain: %w: %s", err, keychainPromptHint)
		}
		msg := bytes.TrimSpace(stderr)
		if len(msg) == 0 {
			return fmt.Errorf("updating Codex credentials in Keychain: %w", err)
		}
		return fmt.Errorf("updating Codex credentials in Keychain: %s", msg)
	}
	return nil
}

func runSecurity(parent context.Context, args ...string) (stdout, stderr []byte, err error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, securityReadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errors.Is(ctx.Err(), context.Canceled) {
		return outBuf.Bytes(), errBuf.Bytes(), context.Canceled
	}
	if ctx.Err() != nil {
		return outBuf.Bytes(), errBuf.Bytes(), errKeychainTimeout
	}
	return outBuf.Bytes(), errBuf.Bytes(), err
}
