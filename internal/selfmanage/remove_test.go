package selfmanage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveDeletesBinaryAndEmptyDedicatedDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "proton-cli")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(sub, "proton-cli")
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Remove(bin, sub); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Errorf("binary still present: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("empty dedicated dir was not removed: %v", err)
	}
}

func TestRemoveKeepsSharedDir(t *testing.T) {
	dir := t.TempDir() // stands in for a shared bin dir like ~/.local/bin
	bin := filepath.Join(dir, "proton-cli")
	sibling := filepath.Join(dir, "other-tool")
	for _, f := range []string{bin, sibling} {
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := Remove(bin, ""); err != nil { // "" => never touch the parent dir
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Errorf("binary still present")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("sibling file was removed: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("shared dir was removed: %v", err)
	}
}
