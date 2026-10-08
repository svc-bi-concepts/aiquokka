package main

import (
	"bytes"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestGofmt keeps the whole module gofmt-clean: the gate runs only go vet and
// go test, so an unformatted file (antigravity.go, cmd/watch.go once) would
// land unnoticed.
func TestGofmt(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
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
