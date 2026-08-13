package cli

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every request this CLI can send is one the live suite sends.
//
// The live suite is the only thing that would notice Proton changing an answer,
// and it can only notice for requests it actually makes. So the set it reaches is
// recorded - `just coverage` writes tests/api-coverage.golden from a real run -
// and this reads the source to find every request the CLI is able to send, and
// fails on one the suite has never sent.
//
// It is checked here rather than by the suite because it needs no account and no
// network: what the CLI can send is a property of the code. The other half, what
// the suite did send, needs a run against Proton, which is why it is a recorded
// file rather than a check.
//
// A request that appears here after a change means one of two things: a test for
// it is missing, or it belongs in unreachable below with the reason nobody can
// reach it.

// coverageGolden is what a live run reached, recorded by `just coverage`.
const coverageGolden = "../../tests/api-coverage.golden"

// unresolved stands for a part of a path that is not known until the command
// runs.
const unresolved = "\x00"

// unreachable are the requests the suite cannot make, and why. They are the
// exceptions that have to be argued for, not a list to grow when a test is
// inconvenient to write.
var unreachable = map[string]string{
	"DELETE /auth/v4/sessions":          "revoking every other session would end the run",
	"DELETE /auth/v4/sessions/{id}":     "the only session there is to revoke is the one running",
	"DELETE /core/v4/labels/{id}":       "removing a contact group, which needs a paid plan",
	"GET /core/v4/keys/salts":           "only a first unlock derives the key password, and the suite resumes a session",
	"PUT /auth/v4/sessions/local/key":   "written once, at the first unlock, before the suite runs",
	"PUT /contacts/v4/contacts/label":   "contact groups need a paid plan and these accounts are free",
	"PUT /contacts/v4/contacts/unlabel": "contact groups need a paid plan and these accounts are free",
}

// untested are the requests a run could make and does not. Each is a gap somebody
// chose to leave, so it is named here and reported on every run rather than
// passing quietly. The list is something to shorten.
var untested = map[string]string{
	"DELETE /drive/volumes/{id}/trash":                          "emptying the drive trash is done only by the seed, which is not traced",
	"GET /calendar/v1/{id}/events/{id}/attendees":               "reached only by an invitation with more attendees than one page holds",
	"POST /drive/shares/{id}/files/{id}/revisions/{id}/restore": "restoring an old revision has no test",
	"POST /pass/v1/share/{id}/alias/custom":                     "making an alias has no test; only the options one can be made from are read",
	"POST /drive/v2/shares/invitations/{id}/reject":             "declining a share is only dry-run tested; accepting has the round trip",
	"PUT /drive/shares/{id}/urls/{id}":                          "changing an existing public link, rather than making one, has no test",
	"PUT /mail/v4/conversations/delete":                         "deleting a whole thread has no test; the message-level one does",
	"PUT /mail/v4/conversations/read":                           "marking a whole thread read has no test",
	"PUT /mail/v4/conversations/unlabel":                        "removing a label from a whole thread has no test",
	"PUT /mail/v4/conversations/unread":                         "marking a whole thread unread has no test",
}

func TestEveryRequestTheCLICanSendIsOneTheSuiteSends(t *testing.T) {
	exercised, err := readCoverage(coverageGolden)
	if err != nil {
		t.Fatalf("read the recorded API surface: %v\n\nRun `just coverage` to record it.", err)
	}
	emitted := emittableRequests(t)
	if len(emitted) == 0 {
		t.Fatal("found no requests in the source; the extractor is broken")
	}

	var missing, known []string
	for _, req := range emitted {
		switch {
		case exercised[req], unreachable[req] != "":
		case untested[req] != "":
			known = append(known, req)
		default:
			missing = append(missing, req)
		}
	}
	sort.Strings(missing)
	sort.Strings(known)
	for _, req := range missing {
		t.Errorf("the CLI can send %s but the live suite never has;\n"+
			"\twrite a test that reaches it, or say why nobody can in `unreachable`,\n"+
			"\tor record it in `untested` if it is a gap you are leaving open", req)
	}
	// The gaps already known about are said out loud on every run, so the list is
	// something to shorten rather than somewhere to put things.
	for _, req := range known {
		t.Logf("not covered by the live suite: %s (%s)", req, untested[req])
	}

}

func readCoverage(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, scanner.Err()
}

// emittableRequests reads every proton.Request the CLI builds and renders it as a
// method and a path template.
//
// The raw `api` command is left out on purpose: its whole purpose is to send a
// request nothing else models, so it can emit anything and covers nothing.
func emittableRequests(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	roots := []string{"../account", "../service", "../proton", "../selfmanage"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			for _, req := range requestsIn(t, path) {
				seen[req] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	out := make([]string, 0, len(seen))
	for req := range seen {
		out = append(out, req)
	}
	sort.Strings(out)
	return out
}

func requestsIn(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	consts := constantsIn(file)

	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isRequestLit(lit) {
			return true
		}
		var method, p string
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Method":
				method = strings.ToUpper(strings.Trim(render(kv.Value, consts), `"`))
			case "Path":
				p = render(kv.Value, consts)
			}
		}
		// A path that is a variable in its entirety names no endpoint that can be
		// read off the source - the two there are, a cross-table probe and the
		// settings tables, are built elsewhere and covered by the commands that use
		// them. Reading source has limits, which is why the other half of this is a
		// recording of a real run.
		if method == "" || p == "" || p == unresolved || strings.Contains(method, unresolved) {
			return true
		}
		out = append(out, method+" "+template(p))
		return true
	})
	return out
}

func isRequestLit(lit *ast.CompositeLit) bool {
	switch t := lit.Type.(type) {
	case *ast.Ident:
		return t.Name == "Request"
	case *ast.SelectorExpr:
		return t.Sel.Name == "Request"
	}
	return false
}

// constantsIn collects the file's own string constants, so a path written as a
// name resolves to what it holds.
func constantsIn(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if s, ok := literal(vs.Values[i]); ok {
					out[name.Name] = s
				}
			}
		}
	}
	return out
}

func literal(e ast.Expr) (string, bool) {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	return s, err == nil
}

// render turns a path expression into a string, with anything that is not known
// at compile time standing in as a placeholder.
func render(e ast.Expr, consts map[string]string) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if s, ok := literal(v); ok {
			return s
		}
	case *ast.Ident:
		if s, ok := consts[v.Name]; ok {
			return s
		}
		return unresolved
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			return render(v.X, consts) + render(v.Y, consts)
		}
	case *ast.CallExpr:
		// fmt.Sprintf("/a/%s/b", …) - the format is the shape.
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" && len(v.Args) > 0 {
			if s, ok := literal(v.Args[0]); ok {
				return s
			}
		}
	}
	return unresolved
}

// template normalises a rendered path: whatever was not a literal, and whatever a
// format string left a verb for, is the same placeholder an exercised path is
// reduced to.
func template(p string) string {
	for _, verb := range []string{"%s", "%d", "%v", "%q"} {
		p = strings.ReplaceAll(p, verb, unresolved)
	}
	segments := strings.Split(p, "/")
	for i, s := range segments {
		if strings.Contains(s, unresolved) {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}
