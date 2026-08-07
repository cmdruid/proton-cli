package drive

import (
	stdctx "context"
	"path/filepath"
	"strings"
	"time"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	drivesvc "github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// The Drive filters, shared by move, copy, trash and delete.
//
// One selection model per collection: every verb that acts on many things accepts
// the same way of saying which, so a filtered move is no harder than a filtered
// trash.

type filters struct {
	pattern     string
	largerThan  string
	smallerThan string
	scope       string
	age         kit.Range
	recursive   bool
	all         bool
}

func (f *filters) register(c *cobra.Command) {
	fl := c.Flags()
	fl.StringVar(&f.pattern, "pattern", "", "Match names against a shell glob, e.g. *.tmp")
	fl.StringVar(&f.largerThan, "larger-than", "", "Match files above SIZE (e.g. 100MB, 2GB)")
	fl.StringVar(&f.smallerThan, "smaller-than", "", "Match files below SIZE")
	fl.StringVar(&f.scope, "scope", "", "Look only inside this folder (default: the whole drive)")
	f.age.Register(fl, "files")
	fl.BoolVar(&f.recursive, "recursive", false, "Descend into subfolders when filtering")
	fl.BoolVar(&f.all, "all", false, "Confirm that no narrowing filter means everything in scope")
}

func (f *filters) set() bool {
	return f.pattern != "" || f.largerThan != "" || f.smallerThan != "" ||
		f.scope != "" || f.age.Set() || f.all
}

// unbounded reports whether --all was given with nothing to narrow it.
func (f *filters) unbounded() bool {
	return f.all && f.pattern == "" && f.largerThan == "" && f.smallerThan == "" &&
		f.scope == "" && !f.age.Set()
}

const filterHint = "--pattern, --larger-than, --older-than or --scope"

// selectItems resolves what a bulk verb should act on: the paths named, plus
// whatever the filters matched under --scope.
func selectItems(c *kit.Invocation, dc *drivesvc.Context, f *filters) (kit.Selection[drivesvc.Child], error) {
	if f.unbounded() {
		if err := kit.Confirm(c, "--all with no other filter covers your whole drive. Continue?"); err != nil {
			return kit.Selection[drivesvc.Child]{}, err
		}
	}
	sel := kit.Selector[drivesvc.Child]{
		Noun:       "items",
		Columns:    childColumns(),
		IDOf:       func(ch drivesvc.Child) string { return ch.LinkID },
		FilterHint: filterHint,
		Scope:      "a whole subtree",
		ByRef: func(ctx stdctx.Context, ref string) (drivesvc.Child, error) {
			res, err := c.App.Drive.ResolvePath(ctx, dc, ref)
			if err != nil {
				return drivesvc.Child{}, err
			}
			kind := 2
			if res.IsFolder {
				kind = 1
			}
			return drivesvc.Child{
				LinkID: res.LinkID, Name: filepath.Base(ref),
				Path: normalise(ref), Type: kind,
			}, nil
		},
	}
	if f.set() {
		sel.ByFilter = func(ctx stdctx.Context) ([]drivesvc.Child, error) {
			return matchItems(ctx, c, dc, f)
		}
	}
	return kit.Select(c, sel)
}

// matchItems walks the scope and keeps what every given filter accepts.
func matchItems(ctx stdctx.Context, c *kit.Invocation, dc *drivesvc.Context, f *filters) ([]drivesvc.Child, error) {
	root := f.scope
	if root == "" {
		root = "/"
	}

	var minSize, maxSize int64
	if f.largerThan != "" {
		n, err := units.ParseSize(f.largerThan)
		if err != nil {
			return nil, kit.Fail("--larger-than: %v", err)
		}
		minSize = n
	}
	if f.smallerThan != "" {
		n, err := units.ParseSize(f.smallerThan)
		if err != nil {
			return nil, kit.Fail("--smaller-than: %v", err)
		}
		maxSize = n
	}
	var olderThan, newerThan int64
	if f.age.OlderThan != "" {
		d, err := units.ParseDuration(f.age.OlderThan)
		if err != nil {
			return nil, kit.Fail("--older-than: %v", err)
		}
		olderThan = time.Now().Add(-d).Unix()
	}
	if f.age.NewerThan != "" {
		d, err := units.ParseDuration(f.age.NewerThan)
		if err != nil {
			return nil, kit.Fail("--newer-than: %v", err)
		}
		newerThan = time.Now().Add(-d).Unix()
	}

	children, err := c.App.Drive.Walk(ctx, dc, root)
	if err != nil {
		return nil, err
	}

	// A size or age filter is a question about a file, so a folder cannot answer
	// it and is left alone rather than swept up.
	fileOnly := minSize > 0 || maxSize > 0 || olderThan != 0 || newerThan != 0

	out := make([]drivesvc.Child, 0, len(children))
	for _, ch := range children {
		if !f.recursive && depthBelow(root, ch.Path) > 1 {
			continue
		}
		isFile := ch.Type != 1
		if fileOnly && !isFile {
			continue
		}
		if f.pattern != "" && !matchGlob(f.pattern, ch.Name) {
			continue
		}
		if minSize > 0 && ch.Size < minSize {
			continue
		}
		if maxSize > 0 && ch.Size > maxSize {
			continue
		}
		if olderThan != 0 && ch.ModifyTime > olderThan {
			continue
		}
		if newerThan != 0 && ch.ModifyTime < newerThan {
			continue
		}
		out = append(out, ch)
	}
	return out, nil
}

// depthBelow counts how many path segments separate p from root, so a
// non-recursive filter can stay on the level it was pointed at.
func depthBelow(root, p string) int {
	rest := strings.TrimPrefix(p, normalise(root))
	return strings.Count(strings.Trim(rest, "/"), "/") + 1
}

// normalise gives a Drive path one spelling, so comparing two of them is
// meaningful.
func normalise(p string) string {
	p = "/" + strings.Trim(p, "/")
	if p == "/" {
		return p
	}
	return p
}

// matchGlob reports whether name matches the shell-style pattern. An empty
// pattern matches everything, so callers need not special-case it.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}
