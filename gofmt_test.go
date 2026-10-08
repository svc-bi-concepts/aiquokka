package main

import (
	"bytes"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestInternalGofmt keeps internal/ gofmt-clean: the gate runs only go vet and
// go test, so an unformatted file (antigravity.go once) would land unnoticed.
func TestInternalGofmt(t *testing.T) {
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := format.Source(src)
		if err != nil {
			return err
		}
		if !bytes.Equal(src, got) {
			t.Errorf("%s is not gofmt-formatted; run gofmt -w %s", path, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
