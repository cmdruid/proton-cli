package render

import "fmt"

// PaginationFooter returns a human-readable one-line footer for a paginated
// list. Page is 0-based to match the --page flag. Returns "" when total is 0.
//
//	total=12,  page=0, pageSize=25 → "12 messages total (single page)."
//	total=312, page=0, pageSize=25 → "Showing 25 of 312 total. Pass --page 1 for the next page."
//	total=312, page=12, pageSize=25 → "Showing 12 of 312 total. (last page)"
//
// The noun is the user-visible plural ("messages", "conversations", …); when
// total == 1 the trailing "s" is stripped for the single-page wording.
func PaginationFooter(noun string, total, page, pageSize, shown int) string {
	if total <= 0 {
		return ""
	}
	if total <= pageSize {
		word := noun
		if total == 1 {
			word = singular(noun)
		}
		return fmt.Sprintf("%d %s total (single page).", total, word)
	}
	if (page+1)*pageSize >= total {
		return fmt.Sprintf("Showing %d of %d total. (last page)", shown, total)
	}
	return fmt.Sprintf("Showing %d of %d total. Pass --page %d for the next page.",
		shown, total, page+1)
}

// SearchFooter returns the footer for a search result set. limit is the value
// the caller passed via --limit; when len(results) == limit we assume the
// server may have more.
//
//	total=0  → "No results."
//	total<limit → "12 results."
//	total==limit → "12 results (limited; raise --limit to see more)."
func SearchFooter(total, limit int) string {
	switch {
	case total <= 0:
		return "No results."
	case total >= limit && limit > 0:
		return fmt.Sprintf("%d results (limited; raise --limit to see more).", total)
	default:
		return fmt.Sprintf("%d results.", total)
	}
}

// singular trims a trailing "s" from noun. Cheap; works for "messages",
// "conversations", "items". Callers pass the plural form.
func singular(noun string) string {
	if n := len(noun); n > 0 && noun[n-1] == 's' {
		return noun[:n-1]
	}
	return noun
}
