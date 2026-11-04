package sitenav

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSectionRoot(t *testing.T) {
	for _, dir := range []string{"output/site/maps", "output/site/sprites/", "./output/site/maps"} {
		if root, ok := SectionRoot(dir); !ok || root != DefaultRoot {
			t.Errorf("SectionRoot(%q) = %q, %v; want %q, true", dir, root, ok, DefaultRoot)
		}
	}
	for _, dir := range []string{"output/maps", "output/site/other", "output/site", "/tmp/maps"} {
		if _, ok := SectionRoot(dir); ok {
			t.Errorf("SectionRoot(%q) accepted a directory outside the site", dir)
		}
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
