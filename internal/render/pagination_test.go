package render

import "testing"

func TestPaginationFooter(t *testing.T) {
	tests := []struct {
		name                     string
		noun                     string
		total, page, pageSize, n int
		want                     string
	}{
		{"zero results", "messages", 0, 0, 25, 0, ""},
		{"single page exact", "messages", 25, 0, 25, 25, "25 messages total (single page)."},
		{"single page under", "messages", 12, 0, 25, 12, "12 messages total (single page)."},
		{"single page singular", "messages", 1, 0, 25, 1, "1 message total (single page)."},
		{"single page no-s noun", "items", 1, 0, 25, 1, "1 item total (single page)."},
		{"mid-pagination first page", "messages", 312, 0, 25, 25,
			"Showing 25 of 312 total. Pass --page 1 for the next page."},
		{"mid-pagination middle", "messages", 312, 4, 25, 25,
			"Showing 25 of 312 total. Pass --page 5 for the next page."},
		{"last page exact boundary", "messages", 50, 1, 25, 25,
			"Showing 25 of 50 total. (last page)"},
		{"last page partial", "messages", 312, 12, 25, 12,
			"Showing 12 of 312 total. (last page)"},
		{"conversations noun", "conversations", 100, 0, 50, 50,
			"Showing 50 of 100 total. Pass --page 1 for the next page."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PaginationFooter(tc.noun, tc.total, tc.page, tc.pageSize, tc.n)
			if got != tc.want {
				t.Errorf("PaginationFooter(%q,%d,%d,%d,%d):\ngot:  %q\nwant: %q",
					tc.noun, tc.total, tc.page, tc.pageSize, tc.n, got, tc.want)
			}
		})
	}
}

func TestSearchFooter(t *testing.T) {
	tests := []struct {
		name         string
		total, limit int
		want         string
	}{
		{"zero results", 0, 25, "No results."},
		{"under limit", 12, 25, "12 results."},
		{"at limit", 25, 25, "25 results (limited; raise --limit to see more)."},
		{"over limit (defensive)", 30, 25, "30 results (limited; raise --limit to see more)."},
		{"zero limit treated as no cap", 5, 0, "5 results."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SearchFooter(tc.total, tc.limit)
			if got != tc.want {
				t.Errorf("SearchFooter(%d,%d) = %q, want %q",
					tc.total, tc.limit, got, tc.want)
			}
		})
	}
}
