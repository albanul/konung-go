//go:build !js

package gamefs

import (
	"path/filepath"
	"testing"
)

func TestReadWriteFold(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(filepath.Join(dir, "questr.RES"), []byte("q")); err != nil {
		t.Fatal(err)
	}
	if !Exists(filepath.Join(dir, "questr.RES")) || Exists(dir) || Exists(filepath.Join(dir, "x")) {
		t.Fatal("Exists is wrong")
	}
	data, err := ReadFileFold(dir, "QUESTR.RES")
	if err != nil || string(data) != "q" {
		t.Fatalf("ReadFileFold = %q, %v", data, err)
	}
	if _, err := ReadFileFold(dir, "MISSING.RES"); err == nil {
		t.Fatal("missing file read")
	}
}
