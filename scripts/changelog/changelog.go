package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// categories are Keep a Changelog's six types of changes, in the order the
// specification lists them. The order is not alphabetical and is not ours to pick.
var categories = []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}

var (
	releaseHeading = regexp.MustCompile(`^## \[([^\]]+)\] - (\d{4}-\d{2}-\d{2})( \[YANKED\])?$`)
	linkReference  = regexp.MustCompile(`^\[[^\]]+\]: \S+$`)
	versionPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z.-]+))?$`)
)

// release is one version's section: what it is called, when it shipped, and the
// bullets that go onto its release page verbatim.
type release struct {
	version string
	date    string
	yanked  bool
	body    string
	heading int
}

// changelog is a parsed CHANGELOG.md, newest release first.
type changelog struct {
	releases []release
}

// releasable is the version the file asks for: the newest one, unless it was
// yanked. A yank withdraws a release, so republishing it is the one thing a rule
// that converges on the file must never do.
func (c *changelog) releasable() (release, bool) {
	if len(c.releases) == 0 || c.releases[0].yanked {
		return release{}, false
	}
	return c.releases[0], true
}

// parse reads a changelog and holds it to Keep a Changelog 1.1.0. It is strict
// where the specification is only principled - category order, no empty sections,
// versions that move one step at a time - because this file decides what gets
// published, and a rule nothing enforces is a rule that drifts.
//
// An `[Unreleased]` section is allowed and never releasable, but not required:
// here a section is written when a release is cut, so between releases the file
// has nothing to say.
func parse(path string, source []byte) (*changelog, error) {
	p := &parser{path: path, category: -1}
	lines := strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "# Changelog" {
		return nil, p.at(0, `the file must start with "# Changelog"`)
	}
	for n, line := range lines {
		if err := p.read(n, line); err != nil {
			return nil, err
		}
	}
	return p.done()
}

type parser struct {
	path     string
	releases []release
	linked   bool

	unreleased bool
	name       string
	heading    int
	body       []string
	category   int
	categoryAt int
	bullets    int
	current    *release
}

func (p *parser) read(n int, line string) error {
	switch {
	case linkReference.MatchString(line):
		if err := p.close(); err != nil {
			return err
		}
		p.linked = true
	case strings.HasPrefix(line, "## "):
		return p.section(n, line)
	case p.name == "":
		return nil
	case strings.HasPrefix(line, "### "):
		return p.begin(n, strings.TrimPrefix(line, "### "))
	case strings.HasPrefix(line, "- "):
		return p.entry(n, line)
	case strings.TrimSpace(line) == "":
		p.body = append(p.body, line)
	case strings.HasPrefix(line, "  ") && p.bullets > 0:
		p.body = append(p.body, line)
	default:
		return p.at(n, "unexpected line in [%s]: %q", p.name, line)
	}
	return nil
}

func (p *parser) section(n int, line string) error {
	if p.linked {
		return p.at(n, "version heading below the link references")
	}
	if err := p.close(); err != nil {
		return err
	}
	if line == "## [Unreleased]" {
		if p.unreleased {
			return p.at(n, "a second [Unreleased] section")
		}
		if len(p.releases) > 0 {
			return p.at(n, "[Unreleased] must sit above every version")
		}
		p.unreleased, p.name, p.heading = true, "Unreleased", n
		return nil
	}
	match := releaseHeading.FindStringSubmatch(line)
	if match == nil {
		return p.at(n, `expected "## [X.Y.Z] - YYYY-MM-DD", got %q`, line)
	}
	version, date := match[1], match[2]
	if version == "Unreleased" {
		return p.at(n, "[Unreleased] never carries a date")
	}
	if !versionPattern.MatchString(version) {
		return p.at(n, "[%s] is not a semantic version", version)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return p.at(n, "[%s] is dated %s, which is not a day", version, date)
	}
	p.name, p.heading = version, n
	p.current = &release{version: version, date: date, yanked: match[3] != "", heading: n}
	return nil
}

func (p *parser) begin(n int, name string) error {
	next := slices.Index(categories, name)
	if next < 0 {
		return p.at(n, "%q is not one of %s", name, strings.Join(categories, ", "))
	}
	if p.category >= 0 && p.bullets == 0 {
		return p.at(p.categoryAt, "%s in [%s] has no entries", categories[p.category], p.name)
	}
	if next <= p.category {
		return p.at(n, "%s in [%s] belongs above %s: the order is %s",
			name, p.name, categories[p.category], strings.Join(categories, ", "))
	}
	p.category, p.categoryAt, p.bullets = next, n, 0
	p.body = append(p.body, "### "+name)
	return nil
}

func (p *parser) entry(n int, line string) error {
	if p.category < 0 {
		return p.at(n, "entry in [%s] sits outside a category", p.name)
	}
	if strings.TrimSpace(strings.TrimPrefix(line, "- ")) == "" {
		return p.at(n, "empty entry in [%s]", p.name)
	}
	p.bullets++
	p.body = append(p.body, line)
	return nil
}

// close finishes the section being read. An empty [Unreleased] is a section with
// nothing in it yet; an empty version section is a release with nothing to say,
// which means the wrong thing was published.
func (p *parser) close() error {
	if p.name == "" {
		return nil
	}
	if p.category >= 0 && p.bullets == 0 {
		return p.at(p.categoryAt, "%s in [%s] has no entries", categories[p.category], p.name)
	}
	if p.current != nil {
		body := trimBlank(p.body)
		if len(body) == 0 {
			return p.at(p.heading, "[%s] has no entries", p.name)
		}
		p.current.body = strings.Join(body, "\n")
		p.releases = append(p.releases, *p.current)
	}
	p.name, p.body, p.category, p.bullets, p.current = "", nil, -1, 0, nil
	return nil
}

func (p *parser) done() (*changelog, error) {
	if err := p.close(); err != nil {
		return nil, err
	}
	for i := 1; i < len(p.releases); i++ {
		newer, older := p.releases[i-1], p.releases[i]
		if semver.Compare("v"+newer.version, "v"+older.version) <= 0 {
			return nil, p.at(newer.heading, "[%s] is not newer than [%s]: the latest version comes first",
				newer.version, older.version)
		}
		if !follows(older.version, newer.version) {
			return nil, p.at(newer.heading, "[%s] does not follow [%s], which is followed by %s",
				newer.version, older.version, strings.Join(successors(older.version), " or "))
		}
	}
	return &changelog{releases: p.releases}, nil
}

func (p *parser) at(n int, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", p.path, n+1, fmt.Sprintf(format, args...))
}

// successors are the versions semantic versioning allows after this one. There
// are three, which is what makes a slip of the finger catchable: 2.30.0 is not
// one of them, and a version once tagged cannot be taken back.
func successors(version string) []string {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return nil
	}
	major, minor, patch := number(match, 1), number(match, 2), number(match, 3)
	core := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if match[4] != "" {
		return []string{core, core + "-<pre-release>"}
	}
	return []string{
		fmt.Sprintf("%d.%d.%d", major, minor, patch+1),
		fmt.Sprintf("%d.%d.0", major, minor+1),
		fmt.Sprintf("%d.0.0", major+1),
	}
}

func follows(previous, next string) bool {
	match := versionPattern.FindStringSubmatch(next)
	if match == nil {
		return false
	}
	core := fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3])
	if before := versionPattern.FindStringSubmatch(previous); before != nil && before[4] != "" {
		if core == fmt.Sprintf("%s.%s.%s", before[1], before[2], before[3]) {
			return true
		}
	}
	return slices.Contains(successors(previous), core)
}

func number(match []string, i int) int {
	n, _ := strconv.Atoi(match[i])
	return n
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
