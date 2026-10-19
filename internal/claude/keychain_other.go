//go:build !darwin

package claude

import "context"

func loadKeychainCredentials(context.Context) ([]byte, error) {
	return nil, errKeychainCredentialsNotFound
}
