package ref

import (
	"errors"
	"testing"

	"github.com/cmdruid/proton-cli/internal/errs"
)

type row struct{ id, label string }

func idOf(r row) string    { return r.id }
func labelOf(r row) string { return r.label }

func TestPickNone(t *testing.T) {
	_, err := Pick("contact", "nobody", []row(nil), idOf, labelOf)
	var nf *errs.NotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want *errs.NotFound", err)
	}
	if nf.Kind != "contact" || nf.Ref != "nobody" {
		t.Errorf("NotFound = %+v, want kind=contact ref=nobody", nf)
	}
}

func TestPickOne(t *testing.T) {
	got, err := Pick("contact", "jane", []row{{id: "ID1", label: "Jane"}}, idOf, labelOf)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.id != "ID1" {
		t.Errorf("got %+v, want the sole match", got)
	}
}

func TestPickMany(t *testing.T) {
	_, err := Pick("contact", "j", []row{{"ID1", "Jane"}, {"ID2", "John"}}, idOf, labelOf)
	var amb *errs.Ambiguous
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want *errs.Ambiguous", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(amb.Candidates))
	}
	if amb.Candidates[0].ID != "ID1" || amb.Candidates[0].Label != "Jane" {
		t.Errorf("candidate[0] = %+v", amb.Candidates[0])
	}
	if amb.ExitCode() != 4 {
		t.Errorf("Ambiguous exit = %d, want 4", amb.ExitCode())
	}
}
