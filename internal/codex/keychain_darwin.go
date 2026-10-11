//go:build darwin

package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// securityTimeout bounds each /usr/bin/security call: a Keychain permission
// dialog on the desktop would otherwise block a run started over SSH.
const securityTimeout = 3 * time.Second

// Codex CLI writes the item with /usr/bin/security, so reading through the same
// helper avoids the permission dialog that Security.framework shows for the
// aiquokka binary (the same approach as the Claude Keychain reader).
func loadKeychainItem(ctx context.Context, account string) ([]byte, error) {
	stdout, stderr, err := runSecurity(ctx, "find-generic-password", "-s", codexKeychainService, "-a", account, "-w")
	if err != nil {
		return nil, classifySecurityError(err, string(stderr))
	}
	data := bytes.TrimSpace(stdout)
	if len(data) == 0 {
		return nil, errKeychainNotFound
	}
	return data, nil
}

// storeKeychainItem replaces the login in place (-U), keeping the item's
// existing access list.
func storeKeychainItem(ctx context.Context, account string, data []byte) error {
	_, stderr, err := runSecurity(ctx, "add-generic-password", "-U", "-s", codexKeychainService, "-a", account, "-w", string(data))
	if err != nil {
		return fmt.Errorf("updating Codex credentials in Keychain: %w", classifySecurityError(err, string(stderr)))
	}
	return nil
}

func runSecurity(parent context.Context, args ...string) (stdout, stderr []byte, err error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, securityTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, nil, errKeychainTimeout
	}
	return outBuf.Bytes(), errBuf.Bytes(), err
}
