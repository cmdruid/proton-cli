package cli

import (
	"fmt"
	"strings"
)

// accentColors is Proton's fixed label/calendar color palette, mirroring
// ACCENT_COLORS in WebClients (packages/shared/lib/colors.ts). The API rejects
// colors outside this set, so the CLI validates client-side for a clear error.
var accentColors = []struct{ name, hex string }{
	{"purple", "#8080FF"}, {"pink", "#DB60D6"}, {"strawberry", "#EC3E7C"},
	{"carrot", "#F78400"}, {"sahara", "#936D58"}, {"enzian", "#5252CC"},
	{"plum", "#A839A4"}, {"cerise", "#BA1E55"}, {"copper", "#C44800"},
	{"soil", "#54473F"}, {"slateblue", "#415DF0"}, {"pacific", "#179FD9"},
	{"reef", "#1DA583"}, {"fern", "#3CBB3A"}, {"olive", "#B4A40E"},
	{"cobalt", "#273EB2"}, {"ocean", "#0A77A6"}, {"pine", "#0F735A"},
	{"forest", "#258723"}, {"pickle", "#807304"},
}

// validateAccentColor returns an error naming the valid palette when color is
// not one of Proton's accent colors (case-insensitive). Empty is allowed so
// callers can supply their own default.
func validateAccentColor(color string) error {
	if color == "" {
		return nil
	}
	for _, c := range accentColors {
		if strings.EqualFold(c.hex, color) {
			return nil
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "invalid color %q; Proton accepts only these accent colors:\n", color)
	for _, c := range accentColors {
		fmt.Fprintf(&b, "  %-11s %s\n", c.name, c.hex)
	}
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}
