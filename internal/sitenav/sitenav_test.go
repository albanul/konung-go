package sitenav

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSectionRoot(t *testing.T) {
	if root, ok := SectionRoot("output/site/maps"); !ok || root != "output/site" {
		t.Fatalf("SectionRoot(maps) = %q, %v", root, ok)
	}
	if _, ok := SectionRoot("output/other"); ok {
		t.Fatal("SectionRoot accepted an unknown directory")
	}
}

func TestWriteHub(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maps", "index.html"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteHub(root); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if !strings.Contains(s, `<h2><a href="maps/index.html">Maps</a>`) {
		t.Error("hub does not link the generated maps section")
	}
	if !strings.Contains(s, "Not generated yet") {
		t.Error("hub does not mark the missing sprites section")
	}
}
