package kit

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// AccentColors is Proton's fixed label/calendar color palette, mirroring
// ACCENT_COLORS in WebClients (packages/shared/lib/colors.ts). The API rejects
// colors outside this set, so the CLI validates client-side for a clear error.
var AccentColors = []struct{ name, hex string }{
	{"purple", "#8080FF"}, {"pink", "#DB60D6"}, {"strawberry", "#EC3E7C"},
	{"carrot", "#F78400"}, {"sahara", "#936D58"}, {"enzian", "#5252CC"},
	{"plum", "#A839A4"}, {"cerise", "#BA1E55"}, {"copper", "#C44800"},
	{"soil", "#54473F"}, {"slateblue", "#415DF0"}, {"pacific", "#179FD9"},
	{"reef", "#1DA583"}, {"fern", "#3CBB3A"}, {"olive", "#B4A40E"},
	{"cobalt", "#273EB2"}, {"ocean", "#0A77A6"}, {"pine", "#0F735A"},
	{"forest", "#258723"}, {"pickle", "#807304"},
}

// ValidateAccentColor rejects anything outside Proton's palette, naming the whole
// palette when it does. Empty is allowed so a caller can supply its own default.
//
// The check is local because the API's own refusal says only that the colour was
// invalid, which leaves the user no better off than before.
func ValidateAccentColor(color string) error {
	if color == "" {
		return nil
	}
	for _, c := range AccentColors {
		if strings.EqualFold(c.hex, color) {
			return nil
		}
	}
	lines := []string{"use one of:"}
	for _, c := range AccentColors {
		lines = append(lines, fmt.Sprintf("  %-11s %s", c.name, c.hex))
	}
	return Fail("%q is not a Proton accent color.", color).Hint(lines...)
}

// DefaultAccentColor is the purple Proton offers first, used wherever a colour is
// optional.
const DefaultAccentColor = "#8080FF"

// Color is a flag holding one of Proton's accent colours.
//
// It is declared rather than checked by hand for the same reason an Enum is: the
// palette is fixed, so a wrong value is wrong before anyone signs in, and Run
// refuses it there. Twenty hex codes are too many to list in a flag's help, so
// the domain appears in the error instead - which is where a person who guessed
// wrong is looking.
type Color struct {
	// Name is the flag name, without dashes.
	Name string
	// Usage is the help text.
	Usage string
	// Default is the colour used when the flag is absent. Empty means the colour
	// is optional and its absence means "leave it alone".
	Default string

	target string
}

func (c *Color) Register(cmd *cobra.Command) {
	c.target = c.Default
	usage := c.Usage
	if usage == "" {
		usage = "Accent color, as a hex value"
	}
	cmd.Flags().StringVar(&c.target, c.Name, c.Default, usage)
	_ = cmd.RegisterFlagCompletionFunc(c.Name,
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			out := make([]string, 0, len(AccentColors))
			for _, a := range AccentColors {
				out = append(out, a.hex+"\t"+a.name)
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		})
	registerCheck(cmd, c.Name, nil, c)
}

// Value returns the validated colour, or "" when none was given.
func (c *Color) Value() string { return c.target }

// Set reports whether a colour was supplied.
func (c *Color) Set() bool { return c.target != "" }

func (c *Color) validate() error { return ValidateAccentColor(c.target) }
