//go:build !darwin

package codex

import "context"

func loadKeychainAuth(context.Context) ([]byte, []byte, error) {
	return nil, nil, errKeychainCredentialsNotFound
}

func loadKeychainItem(context.Context, []byte) ([]byte, error) {
	return nil, errKeychainCredentialsNotFound
}

func persistKeychainAuth(context.Context, []byte, []byte) error {
	return errKeychainCredentialsNotFound
}

// keychainMention keeps the not-configured message unchanged off darwin.
const keychainMention = ""
