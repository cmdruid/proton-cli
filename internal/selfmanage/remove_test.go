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
	bin := filepath.Join(sub, "proton")
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(sub, AliasFiles("proton-cli")[0])
	if err := LinkAlias(bin, alias); err != nil {
		t.Fatal(err)
	}
	if err := Remove(bin, []string{alias}, sub); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(alias); !os.IsNotExist(err) {
		t.Errorf("alias still present: %v", err)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Errorf("binary still present: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("empty dedicated dir was not removed: %v", err)
	}
}

// An alias resolves to the program beside it, so a directory that is moved or
// installed from an archive still answers to both names.
func TestLinkAliasResolvesToTheProgram(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "proton")
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, AliasFiles("proton-cli")[0])
	if err := LinkAlias(bin, alias); err != nil {
		t.Fatalf("LinkAlias: %v", err)
	}
	// Linking again replaces what is there rather than failing, so an update
	// repairs a stale or dangling name.
	if err := LinkAlias(bin, alias); err != nil {
		t.Fatalf("LinkAlias again: %v", err)
	}
	if _, err := os.Stat(alias); err != nil {
		t.Errorf("alias does not resolve: %v", err)
	}
}

func TestRemoveKeepsSharedDir(t *testing.T) {
	dir := t.TempDir() // stands in for a shared bin dir like ~/.local/bin
	bin := filepath.Join(dir, "proton")
	sibling := filepath.Join(dir, "other-tool")
	for _, f := range []string{bin, sibling} {
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// No alias, and no dedicated dir: nothing but the binary is this install's.
	if err := Remove(bin, nil, ""); err != nil {
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
