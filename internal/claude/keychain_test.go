package claude

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestClassifySecurityErrorNotFoundExitCode(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 44").Run()
	got := classifySecurityError(err, "")
	if !errors.Is(got, errKeychainCredentialsNotFound) {
		t.Fatalf("got %v, want errKeychainCredentialsNotFound", got)
	}
}

func TestClassifySecurityErrorNotFoundMessage(t *testing.T) {
	err := classifySecurityError(errors.New("exit status 1"), "The specified item could not be found in the keychain.")
	if !errors.Is(err, errKeychainCredentialsNotFound) {
		t.Fatalf("got %v, want errKeychainCredentialsNotFound", err)
	}
}

func TestClassifySecurityErrorOther(t *testing.T) {
	err := classifySecurityError(errors.New("exit status 1"), "User interaction is not allowed.")
	if errors.Is(err, errKeychainCredentialsNotFound) {
		t.Fatal("classified a permission error as not found")
	}
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "security unlock-keychain") {
		t.Fatalf("expected unlock-keychain hint, got %v", err)
	}
}

func TestClassifySecurityErrorCanceled(t *testing.T) {
	err := classifySecurityError(context.Canceled, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestClassifySecurityErrorTimeout(t *testing.T) {
	err := classifySecurityError(errKeychainTimeout, "")
	if !errors.Is(err, errKeychainTimeout) {
		t.Fatalf("got %v, want errKeychainTimeout", err)
	}
	if !strings.Contains(err.Error(), "permission dialog") {
		t.Fatalf("expected permission-dialog hint, got %v", err)
	}
}

func TestDecodeSecurityHex(t *testing.T) {
	plainJSON := []byte(`{"claudeAiOauth":{"accessToken":"t"}}`)
	prettyJSON := []byte("{\n  \"claudeAiOauth\": {\n    \"accessToken\": \"t\"\n  }\n}\n")
	hexOfPretty := []byte(hex.EncodeToString(prettyJSON))
	hexOfText := []byte(hex.EncodeToString([]byte("hello")))

	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{"plain JSON passes through", plainJSON, plainJSON},
		{"hex of pretty JSON decodes", hexOfPretty, prettyJSON},
		{"hex that does not decode to JSON stays hex", hexOfText, hexOfText},
		{"empty stays empty", []byte{}, []byte{}},
		{"odd length stays", []byte("7b0a1"), []byte("7b0a1")},
		{"non-hex stays", []byte("7b0g"), []byte("7b0g")},
		{"uppercase hex stays", []byte("7B0A"), []byte("7B0A")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeSecurityHex(tt.in)
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeSecurityHexParsesAsCredentials(t *testing.T) {
	prettyJSON := []byte("{\n  \"claudeAiOauth\": {\n    \"accessToken\": \"tok\",\n    \"refreshToken\": \"ref\"\n  }\n}\n")
	got := decodeSecurityHex([]byte(hex.EncodeToString(prettyJSON)))
	var parsed credentialsFile
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("decoding hex output: %v", err)
	}
	if parsed.ClaudeAiOauth.AccessToken != "tok" || parsed.ClaudeAiOauth.RefreshToken != "ref" {
		t.Fatalf("got %+v, want claudeAiOauth with tok/ref", parsed.ClaudeAiOauth)
	}
}
