//go:build darwin

package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"syscall"
	"time"
)

// securityReadTimeout is how long we wait for /usr/bin/security before
// treating the call as blocked on a GUI dialog.
const securityReadTimeout = 3 * time.Second

// Claude Code writes the Keychain item with /usr/bin/security, which leaves
// that tool on the item ACL. Reading through the same helper avoids the
// permission dialog that Security.framework shows for the aiquokka binary.
// Calls are time-bounded so a dialog over SSH cannot hang the process.
// aiquokka only reads this item: it never writes Claude Code's credentials.
func loadKeychainCredentials(ctx context.Context) ([]byte, error) {
	account := keychainAccount()
	data, err := securityFindPassword(ctx, claudeCredentialsService, account)
	if err != nil && account != "" && !errors.Is(err, errKeychainTimeout) && !errors.Is(err, context.Canceled) {
		data, err = securityFindPassword(ctx, claudeCredentialsService, "")
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func keychainAccount() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

func securityFindPassword(ctx context.Context, service, account string) ([]byte, error) {
	stdout, stderr, err := runSecurity(ctx, findPasswordArgs(service, account, true)...)
	if err != nil {
		return nil, classifySecurityError(err, string(stderr))
	}
	stdout = bytes.TrimSpace(stdout)
	if len(stdout) == 0 {
		return nil, errKeychainCredentialsNotFound
	}
	return decodeSecurityHex(stdout), nil
}

func findPasswordArgs(service, account string, secret bool) []string {
	args := []string{"find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	if secret {
		args = append(args, "-w")
	}
	return args
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
