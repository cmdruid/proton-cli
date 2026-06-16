package mail

import "testing"

func TestFilterInline(t *testing.T) {
	tests := []struct {
		name    string
		in      []Attachment
		wantIDs []string
	}{
		{
			name:    "all attachments kept",
			in:      []Attachment{{ID: "a", Disposition: "attachment"}, {ID: "b", Disposition: "attachment"}},
			wantIDs: []string{"a", "b"},
		},
		{
			name:    "all inline dropped",
			in:      []Attachment{{ID: "i1", Disposition: "inline"}, {ID: "i2", Disposition: "inline"}},
			wantIDs: []string{},
		},
		{
			name: "mixed: inline dropped, attachment kept",
			in: []Attachment{
				{ID: "i", Disposition: "inline"},
				{ID: "a", Disposition: "attachment"},
				{ID: "i2", Disposition: "inline"},
				{ID: "a2", Disposition: "attachment"},
			},
			wantIDs: []string{"a", "a2"},
		},
		{
			name: "missing disposition treated as attachment (kept)",
			in: []Attachment{
				{ID: "x", Disposition: ""},
				{ID: "i", Disposition: "inline"},
				{ID: "y"},
			},
			wantIDs: []string{"x", "y"},
		},
		{
			name:    "unknown disposition treated as attachment (kept)",
			in:      []Attachment{{ID: "weird", Disposition: "embed"}, {ID: "i", Disposition: "inline"}},
			wantIDs: []string{"weird"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterInline(tc.in)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("len = %d, want %d (got: %v)", len(got), len(tc.wantIDs), idsOf(got))
			}
			for i, w := range tc.wantIDs {
				if got[i].ID != w {
					t.Errorf("[%d].ID = %q, want %q", i, got[i].ID, w)
				}
			}
		})
	}
}

func TestAttachmentIsInline(t *testing.T) {
	tests := []struct {
		disposition string
		want        bool
	}{
		{"inline", true},
		{"attachment", false},
		{"", false},
		{"embed", false},
		{"INLINE", false},
	}
	for _, tc := range tests {
		t.Run(tc.disposition, func(t *testing.T) {
			if got := (Attachment{Disposition: tc.disposition}).IsInline(); got != tc.want {
				t.Errorf("IsInline() = %v, want %v", got, tc.want)
			}
		})
	}
}

func idsOf(atts []Attachment) []string {
	out := make([]string, len(atts))
	for i, a := range atts {
		out[i] = a.ID
	}
	return out
}
