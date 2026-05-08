package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitLastDot(t *testing.T) {
	tests := []struct {
		in       string
		wantStem string
		wantExt  string
	}{
		{"file.txt", "file", "txt"},
		{"archive.tar.gz", "archive.tar", "gz"},
		{"Makefile", "Makefile", ""},
		{".bashrc", ".bashrc", ""},
		{"weird.", "weird", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			stem, ext := splitLastDot(tc.in)
			if stem != tc.wantStem || ext != tc.wantExt {
				t.Errorf("splitLastDot(%q) = (%q, %q), want (%q, %q)",
					tc.in, stem, ext, tc.wantStem, tc.wantExt)
			}
		})
	}
}

func TestSuffixedName(t *testing.T) {
	dir := t.TempDir()
	t.Run("no collision returns name_1", func(t *testing.T) {
		// Pre-create the canonical file so SuffixedName has a reason to exist.
		mustWrite(t, filepath.Join(dir, "file.txt"))
		got, err := SuffixedName(filepath.Join(dir, "file.txt"))
		if err != nil {
			t.Fatalf("SuffixedName: %v", err)
		}
		if got != filepath.Join(dir, "file_1.txt") {
			t.Errorf("got %s, want file_1.txt", got)
		}
	})
	t.Run("walks until free slot", func(t *testing.T) {
		base := filepath.Join(dir, "doc.pdf")
		mustWrite(t, base)
		mustWrite(t, filepath.Join(dir, "doc_1.pdf"))
		mustWrite(t, filepath.Join(dir, "doc_2.pdf"))
		got, err := SuffixedName(base)
		if err != nil {
			t.Fatalf("SuffixedName: %v", err)
		}
		if got != filepath.Join(dir, "doc_3.pdf") {
			t.Errorf("got %s, want doc_3.pdf", got)
		}
	})
	t.Run("multi-dot keeps last extension", func(t *testing.T) {
		base := filepath.Join(dir, "archive.tar.gz")
		mustWrite(t, base)
		got, err := SuffixedName(base)
		if err != nil {
			t.Fatalf("SuffixedName: %v", err)
		}
		if got != filepath.Join(dir, "archive.tar_1.gz") {
			t.Errorf("got %s, want archive.tar_1.gz", got)
		}
	})
	t.Run("no extension appends _N", func(t *testing.T) {
		base := filepath.Join(dir, "Makefile")
		mustWrite(t, base)
		got, err := SuffixedName(base)
		if err != nil {
			t.Fatalf("SuffixedName: %v", err)
		}
		if got != filepath.Join(dir, "Makefile_1") {
			t.Errorf("got %s, want Makefile_1", got)
		}
	})
}

func TestWriteFileSafe(t *testing.T) {
	dir := t.TempDir()

	t.Run("WriteError fails if exists", func(t *testing.T) {
		path := filepath.Join(dir, "a.txt")
		mustWrite(t, path)
		_, err := WriteFileSafe(path, []byte("new"), 0644, WriteError)
		if err == nil {
			t.Fatal("expected error on existing path")
		}
		if !strings.Contains(err.Error(), "exists") {
			t.Errorf("error should mention 'exists': %v", err)
		}
	})

	t.Run("WriteAutoSuffix walks past existing", func(t *testing.T) {
		path := filepath.Join(dir, "b.txt")
		mustWrite(t, path)
		written, err := WriteFileSafe(path, []byte("new"), 0644, WriteAutoSuffix)
		if err != nil {
			t.Fatalf("WriteFileSafe: %v", err)
		}
		if written != filepath.Join(dir, "b_1.txt") {
			t.Errorf("written = %s, want b_1.txt", written)
		}
		// Original untouched.
		if data, _ := os.ReadFile(path); string(data) != "original" {
			t.Errorf("original modified: %q", string(data))
		}
		// Suffixed has new content.
		if data, _ := os.ReadFile(written); string(data) != "new" {
			t.Errorf("suffixed missing new content: %q", string(data))
		}
	})

	t.Run("WriteAutoSuffix on free path uses given name", func(t *testing.T) {
		path := filepath.Join(dir, "c.txt")
		written, err := WriteFileSafe(path, []byte("first"), 0644, WriteAutoSuffix)
		if err != nil {
			t.Fatalf("WriteFileSafe: %v", err)
		}
		if written != path {
			t.Errorf("written = %s, want %s", written, path)
		}
	})

	t.Run("WriteForce overwrites", func(t *testing.T) {
		path := filepath.Join(dir, "d.txt")
		mustWrite(t, path)
		written, err := WriteFileSafe(path, []byte("forced"), 0644, WriteForce)
		if err != nil {
			t.Fatalf("WriteFileSafe: %v", err)
		}
		if written != path {
			t.Errorf("written = %s, want %s", written, path)
		}
		if data, _ := os.ReadFile(path); string(data) != "forced" {
			t.Errorf("not overwritten: %q", string(data))
		}
	})
}

// mustWrite creates path with content "original" or fails the test.
func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatalf("setup: write %s: %v", path, err)
	}
}
