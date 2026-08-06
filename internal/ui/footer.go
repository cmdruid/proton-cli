package ui

import (
	"fmt"
	"strings"
)

// FooterSpec describes what a collection actually returned, so one generator can
// word every list's footer. Callers state facts; the wording lives here.
type FooterSpec struct {
	// Noun is the collection's plural name ("messages").
	Noun string
	// Count is how many rows were shown.
	Count int
	// Total is how many exist, or Unknown when the server does not say.
	Total int
	// Page and PageSize describe a paginated request; Page is Unpaged otherwise.
	Page, PageSize int
	// Limit is the cap the caller asked for, when reaching it means more may
	// exist. Zero when the request was not capped.
	Limit int
}

const (
	// Unknown marks a total the server did not report.
	Unknown = -1
	// Unpaged marks a collection that is not paginated.
	Unpaged = -1
)

// Footer renders the one-line summary that follows a table on stderr.
//
//	No messages.
//	12 messages.
//	25 of 312 messages. Next page: --page 1
//	25 of 312 messages. (last page)
//	25 messages. More may exist; raise --limit.
func Footer(s FooterSpec) string {
	if s.Count == 0 {
		return "No " + s.Noun + "."
	}
	paged := s.Page >= 0 && s.PageSize > 0 && s.Total > s.Count
	switch {
	case paged && (s.Page+1)*s.PageSize < s.Total:
		return fmt.Sprintf("%d of %d %s. Next page: --page %d", s.Count, s.Total, s.Noun, s.Page+1)
	case paged:
		return fmt.Sprintf("%d of %d %s. (last page)", s.Count, s.Total, s.Noun)
	case s.Limit > 0 && s.Count >= s.Limit:
		return fmt.Sprintf("%s. More may exist; raise --limit.", Quantity(s.Count, s.Noun))
	}
	return Quantity(s.Count, s.Noun) + "."
}

// Quantity renders a count with its noun agreeing in number: "1 message",
// "3 messages". It replaces the "%d message(s)" shorthand, which reads like a
// form rather than a sentence.
func Quantity(n int, plural string) string {
	if n == 1 {
		return "1 " + Singular(plural)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// Singular derives the singular of a collection noun. Collection names in this
// CLI are ordinary English plurals, so two suffix rules cover all of them:
// "addresses"/"aliases" lose "es", everything else loses "s".
func Singular(plural string) string {
	switch {
	case strings.HasSuffix(plural, "ses"), strings.HasSuffix(plural, "xes"),
		strings.HasSuffix(plural, "ches"), strings.HasSuffix(plural, "shes"):
		return strings.TrimSuffix(plural, "es")
	case strings.HasSuffix(plural, "s"):
		return strings.TrimSuffix(plural, "s")
	}
	return plural
}
