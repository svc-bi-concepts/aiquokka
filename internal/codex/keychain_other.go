//go:build !darwin

package codex

import "context"

func loadKeychainItem(context.Context, string) ([]byte, error) {
	return nil, errKeychainNotFound
}

func storeKeychainItem(context.Context, string, []byte) error {
	return errKeychainNotFound
}
