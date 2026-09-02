package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmdruid/proton-cli/internal/cli/kit"
)

// A help screen is the documentation most people read, and there are 164 of
// them built from one renderer. So they are pinned the way the responses are:
// exact bytes, in a file, for the three shapes a screen can take.
//
// Regenerate with:  just golden      (go test ./internal/cli -update)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestHelpReadsTheSameOnEveryShapeOfCommand(t *testing.T) {
	for _, c := range []struct {
		name string
		path []string
	}{
		// The root, which is where the grammar and the global flags are taught.
		{name: "help_root"},
		// A group, which holds commands and acts on nothing.
		{name: "help_group", path: []string{"mail", "messages"}},
		// A leaf, which is the only shape with flags of its own.
		{name: "help_leaf", path: []string{"mail", "messages", "send"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := newRoot()
			cmd, _, err := root.Find(c.path)
			if err != nil {
				t.Fatalf("find %v: %v", c.path, err)
			}
			var out bytes.Buffer
			writeHelp(&out, cmd)
			checkGolden(t, c.name, out.String())
		})
	}
}

// Every screen points at a heading that the generated reference actually wrote.
//
// A help screen offering a dead link is worse than one offering none, and the
// two are written by different programs from the same function - so this is
// where they are held to having agreed. Regenerating the reference is what fixes
// a failure here, never editing the page.
func TestEveryHelpScreenPointsAtSomethingThatWasWritten(t *testing.T) {
	leaves, groups := partition(t)
	pages := map[string]string{}
	for _, c := range append(leaves, groups...) {
		page := kit.ReferencePage(c)
		if page == "" {
			continue
		}
		if _, seen := pages[page]; !seen {
			src, err := os.ReadFile(filepath.Join("..", "..", "docs", "commands", page+".md"))
			if err != nil {
				t.Errorf("%s points at docs/commands/%s.md, which is not generated (run `just docs`)",
					cmdPath(c), page)
				pages[page] = ""
				continue
			}
			pages[page] = string(src)
		}
		heading := kit.ReferenceHeading(c)
		if heading == "" || pages[page] == "" {
			continue
		}
		// Whatever a heading is decorated with, an anchor is its words hyphenated,
		// which is what both GitHub and the site derive.
		if !strings.Contains(pages[page], "\n## `"+heading+"`\n") &&
			!strings.Contains(pages[page], "\n### `"+heading+"`\n") {
			t.Errorf("%s links to %s, but docs/commands/%s.md has no heading %q (run `just docs`)",
				cmdPath(c), kit.Reference(c), page, heading)
		}
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run `just golden` to create it)", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s\n\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}
