package kit

import (
	"fmt"
	"strings"

	"github.com/cmdruid/proton-cli/internal/ui"
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

// AccentColor resolves a colour the user typed to the hex Proton stores.
//
// Both spellings are accepted, because both are spellings of the same thing: the
// palette has twenty entries and every one of them has a name. A list that shows
// "purple" and a flag that only takes "#8080FF" would be the CLI printing
// something it will not read back, which is the one thing its references are
// meant never to do.
//
// Empty resolves to empty, so a caller can supply its own default.
func AccentColor(color string) (string, error) {
	if color == "" {
		return "", nil
	}
	for _, c := range AccentColors {
		if strings.EqualFold(c.hex, color) || strings.EqualFold(c.name, color) {
			return c.hex, nil
		}
	}
	lines := []string{"use a name or a hex value:"}
	for _, c := range AccentColors {
		lines = append(lines, fmt.Sprintf("  %-11s %s", c.name, c.hex))
	}
	return "", Fail("%q is not a Proton accent color.", color).Hint(lines...)
}

// ValidateAccentColor rejects anything outside Proton's palette, naming the whole
// palette when it does.
//
// The check is local because the API's own refusal says only that the colour was
// invalid, which leaves the user no better off than before.
func ValidateAccentColor(color string) error {
	_, err := AccentColor(color)
	return err
}

// AccentName is Proton's own name for a colour, or "" for a hex outside the
// palette. It mirrors getColorName in WebClients (packages/shared/lib/colors.ts).
func AccentName(hex string) string {
	for _, c := range AccentColors {
		if strings.EqualFold(c.hex, hex) {
			return c.name
		}
	}
	return ""
}

// ColorColumn is the COLOR column, wherever a collection has one.
//
// It shows the colour rather than describing it: a swatch painted in the colour
// itself, and beside it the name Proton uses. A hex code is what the API stores,
// not what a person reads, so it appears only for a value outside the palette -
// where there is no name to give.
//
// Machine output is untouched: the hex is the field, as it always was.
func ColorColumn[T any](hex func(T) string) ui.Column[T] {
	return ui.Column[T]{
		Header: "COLOR",
		Swatch: hex,
		Cell: func(row T) string {
			v := hex(row)
			if name := AccentName(v); name != "" {
				return name
			}
			return v
		},
	}
}

// ColorField is ColorColumn's counterpart for a record.
func ColorField(hex string) ui.Field {
	value := hex
	if name := AccentName(hex); name != "" {
		value = name
	}
	return ui.Field{Label: "Color", Value: value, Swatch: hex}
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
		usage = "Accent color, by name (purple) or hex (#8080FF)"
	}
	cmd.Flags().StringVar(&c.target, c.Name, c.Default, usage)
	_ = cmd.RegisterFlagCompletionFunc(c.Name,
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			// Completing the names rather than the hexes offers the spelling a
			// person can recognise; the hex trails as the description.
			out := make([]string, 0, len(AccentColors))
			for _, a := range AccentColors {
				out = append(out, a.name+"\t"+a.hex)
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		})
	registerCheck(cmd, c.Name, nil, c)
}

// Value returns the colour as Proton stores it, whichever spelling was given, or
// "" when none was.
func (c *Color) Value() string {
	hex, err := AccentColor(c.target)
	if err != nil {
		// Unreachable: validate has already refused anything unresolvable, before
		// the command body ran.
		return c.target
	}
	return hex
}

// Set reports whether a colour was supplied.
func (c *Color) Set() bool { return c.target != "" }

func (c *Color) validate() error { return ValidateAccentColor(c.target) }
