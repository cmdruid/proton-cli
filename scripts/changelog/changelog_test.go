package main

import (
	"os"
	"strings"
	"testing"
)

const shipped = `## [2.3.0] - 2026-08-15

### Added

- Something that did not exist before.

### Fixed

- Something that behaved wrongly.
`

func document(sections string) []byte {
	return []byte("# Changelog\n\nAll notable changes to this project will be documented in this file.\n\n" + sections)
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		sections string
		refuses  string
	}{
		{
			name:     "a released version",
			sections: shipped,
		},
		{
			name:     "nothing released yet",
			sections: "",
		},
		{
			name: "several versions",
			sections: `## [2.3.0] - 2026-08-15

### Added

- Something newer.

## [2.2.3] - 2026-08-12

### Fixed

- Something older.
`,
		},
		{
			name:     "an unreleased section above them",
			sections: "## [Unreleased]\n\n" + shipped,
		},
		{
			name:     "an unreleased section carrying entries",
			sections: "## [Unreleased]\n\n### Added\n\n- Something on its way.\n\n" + shipped,
		},
		{
			name: "link references, for a file that keeps them",
			sections: shipped + `
[2.3.0]: https://example.test/compare/v2.2.3...v2.3.0
`,
		},
		{
			name: "a wrapped entry",
			sections: `## [2.3.0] - 2026-08-15

### Added

- Something that did not exist before, described at a length that the author
  chose to wrap.
`,
		},
		{
			name: "a pre-release and the version it precedes",
			sections: `## [2.3.0] - 2026-08-16

### Added

- The finished thing.

## [2.3.0-rc.1] - 2026-08-15

### Added

- The thing, for testing.
`,
		},
		{
			name:     "a yanked version",
			sections: "## [2.3.0] - 2026-08-15 [YANKED]\n\n### Added\n\n- Something that had to be withdrawn.\n",
		},
		{
			name:     "an unreleased section below a version",
			sections: shipped + "\n## [Unreleased]\n",
			refuses:  "[Unreleased] must sit above every version",
		},
		{
			name:     "an invented category",
			sections: "## [2.3.0] - 2026-08-15\n\n### Breaking\n\n- Something.\n",
			refuses:  `"Breaking" is not one of Added, Changed`,
		},
		{
			name: "categories out of order",
			sections: `## [2.3.0] - 2026-08-15

### Fixed

- Something that behaved wrongly.

### Added

- Something that did not exist before.
`,
			refuses: "Added in [2.3.0] belongs above Fixed",
		},
		{
			name:     "a category with no entries",
			sections: "## [2.3.0] - 2026-08-15\n\n### Added\n\n### Fixed\n\n- Something.\n",
			refuses:  "Added in [2.3.0] has no entries",
		},
		{
			name:     "a version with no entries",
			sections: "## [2.3.0] - 2026-08-15\n",
			refuses:  "[2.3.0] has no entries",
		},
		{
			name:     "an entry outside a category",
			sections: "## [2.3.0] - 2026-08-15\n\n- Something.\n",
			refuses:  "sits outside a category",
		},
		{
			name:     "an undated version",
			sections: "## [2.3.0]\n\n### Added\n\n- Something.\n",
			refuses:  `expected "## [X.Y.Z] - YYYY-MM-DD"`,
		},
		{
			name:     "a day that does not exist",
			sections: "## [2.3.0] - 2026-13-45\n\n### Added\n\n- Something.\n",
			refuses:  "which is not a day",
		},
		{
			name:     "a version that is not one",
			sections: "## [2.3] - 2026-08-15\n\n### Added\n\n- Something.\n",
			refuses:  "[2.3] is not a semantic version",
		},
		{
			name:     "a dated unreleased section",
			sections: "## [Unreleased] - 2026-08-15\n\n### Added\n\n- Something.\n",
			refuses:  "[Unreleased] never carries a date",
		},
		{
			name: "the oldest version first",
			sections: `## [2.2.3] - 2026-08-12

### Fixed

- Something older.

## [2.3.0] - 2026-08-15

### Added

- Something newer.
`,
			refuses: "the latest version comes first",
		},
		{
			name: "a version reached by a typo",
			sections: `## [2.30.0] - 2026-08-15

### Added

- Something.

## [2.2.3] - 2026-08-12

### Fixed

- Something older.
`,
			refuses: "[2.30.0] does not follow [2.2.3], which is followed by 2.2.4 or 2.3.0 or 3.0.0",
		},
		{
			name:     "prose where an entry belongs",
			sections: "## [2.3.0] - 2026-08-15\n\n### Added\n\nSomething, at length.\n",
			refuses:  "unexpected line in [2.3.0]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parse("CHANGELOG.md", document(test.sections))
			switch {
			case test.refuses == "" && err != nil:
				t.Fatalf("refused a conformant changelog: %v", err)
			case test.refuses == "":
			case err == nil:
				t.Fatalf("accepted a changelog it should have refused, expected %q", test.refuses)
			case !strings.Contains(err.Error(), test.refuses):
				t.Fatalf("refused it for the wrong reason:\n got %v\nwant %s", err, test.refuses)
			}
		})
	}
}

func TestParseRejectsAFileThatIsNotAChangelog(t *testing.T) {
	if _, err := parse("CHANGELOG.md", []byte("# Release notes\n")); err == nil {
		t.Fatal("accepted a file that does not start with # Changelog")
	}
}

func TestReleasable(t *testing.T) {
	tests := []struct {
		name     string
		sections string
		want     string
	}{
		{"a released version", shipped, "2.3.0"},
		{"nothing released yet", "", ""},
		{"an unreleased section above them", "## [Unreleased]\n\n" + shipped, "2.3.0"},
		{"only an unreleased section", "## [Unreleased]\n", ""},
		{
			name:     "a yanked version",
			sections: "## [2.3.0] - 2026-08-15 [YANKED]\n\n### Added\n\n- Something.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := parse("CHANGELOG.md", document(test.sections))
			if err != nil {
				t.Fatal(err)
			}
			target, ok := document.releasable()
			if ok != (test.want != "") {
				t.Fatalf("releasable = %v, want %v", ok, test.want != "")
			}
			if target.version != test.want {
				t.Fatalf("version = %q, want %q", target.version, test.want)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	document, err := parse("CHANGELOG.md", document(shipped))
	if err != nil {
		t.Fatal(err)
	}
	target, ok := document.releasable()
	if !ok {
		t.Fatal("nothing to release")
	}
	want := `### Added

- Something that did not exist before.

### Fixed

- Something that behaved wrongly.`
	if target.body != want {
		t.Fatalf("notes:\n got %q\nwant %q", target.body, want)
	}
}

// The repository's own changelog is the release trigger, so it is held to the
// same rules as any other, in a test that runs without credentials or a network.
func TestRepositoryChangelog(t *testing.T) {
	path := "../../CHANGELOG.md"
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parse(path, source); err != nil {
		t.Fatal(err)
	}
}
