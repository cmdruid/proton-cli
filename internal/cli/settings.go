package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/spf13/cobra"
)

// This file owns the account-level `settings` command plus the table-driven
// `set KEY VALUE` machinery every product's settings tree reuses. A scope
// declares a map of keys to specs; parsing, validation, the key listing, the
// help text and shell completion are all derived from that one table, so a key
// can never accept a value its own help does not document.

// enumValue is one named integer a setting accepts, e.g. "conversations" = 0.
// Names exist so callers never have to remember Proton's magic numbers, which
// stay accepted for scripts written against the API directly.
type enumValue struct {
	Name string
	N    int
}

// intRange bounds an integer setting. Unit annotates the rendered domain
// ("0-20 (seconds)").
type intRange struct {
	Min, Max int
	Unit     string
}

// settingSpec describes one writable setting: the endpoint that stores it, the
// request-body field it lands in, the web-settings page it belongs to (used to
// group the key listing the way Proton groups it), and the values it accepts.
//
// At most one value domain is set. All three empty means free text, which is
// right for opaque values such as a locale or an IANA time zone.
type settingSpec struct {
	Path  string
	Field string
	Page  string
	Desc  string

	Enum  []enumValue
	Range *intRange
	Text  []string
}

// onOff is the value domain of a plain 0/1 toggle.
func onOff() []enumValue { return []enumValue{{"off", 0}, {"on", 1}} }

// parse converts a user-supplied value into the JSON value the API expects,
// rejecting anything the spec does not permit. The returned error is phrased to
// read as "<key> <error>", e.g. `view-mode accepts: conversations, messages`.
func (s settingSpec) parse(raw string) (any, error) {
	switch {
	case len(s.Enum) > 0:
		for _, v := range s.Enum {
			if strings.EqualFold(raw, v.Name) {
				return v.N, nil
			}
		}
		if n, err := strconv.Atoi(raw); err == nil {
			for _, v := range s.Enum {
				if v.N == n {
					return n, nil
				}
			}
		}
		return nil, fmt.Errorf("accepts: %s", s.allowed())
	case s.Range != nil:
		n, err := strconv.Atoi(raw)
		if err != nil || n < s.Range.Min || n > s.Range.Max {
			return nil, fmt.Errorf("accepts %s", s.allowed())
		}
		return n, nil
	case len(s.Text) > 0:
		for _, v := range s.Text {
			if strings.EqualFold(raw, v) {
				return v, nil
			}
		}
		return nil, fmt.Errorf("accepts: %s", s.allowed())
	}
	if raw == "" {
		return nil, fmt.Errorf("needs a non-empty value")
	}
	return raw, nil
}

// allowed renders the spec's value domain for help text and error messages.
func (s settingSpec) allowed() string {
	switch {
	case len(s.Enum) > 0:
		return strings.Join(s.completions(), ", ")
	case s.Range != nil:
		if s.Range.Unit != "" {
			return fmt.Sprintf("%d-%d (%s)", s.Range.Min, s.Range.Max, s.Range.Unit)
		}
		return fmt.Sprintf("%d-%d", s.Range.Min, s.Range.Max)
	case len(s.Text) > 0:
		return strings.Join(s.Text, ", ")
	}
	return s.Desc
}

// completions lists the concrete values shell completion should offer. Ranges
// and free text have no finite set, so they offer nothing.
func (s settingSpec) completions() []string {
	switch {
	case len(s.Enum) > 0:
		out := make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			out = append(out, v.Name)
		}
		return out
	case len(s.Text) > 0:
		return s.Text
	}
	return nil
}

// settingsCmd builds a product's settings command: a bare invocation shows the
// current values, `set` writes one. scope names the tree in help and error text
// ("account", "mail", …); show renders the current state.
func settingsCmd(scope, short string, specs map[string]settingSpec, show Handler) *cobra.Command {
	c := &cobra.Command{
		Use:   "settings",
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  run([]Step{stepAuth}, show),
	}
	c.AddCommand(settingsSetCmd(scope, specs))
	return c
}

// settingsSetCmd builds `set KEY VALUE` for one scope. With no arguments it
// prints the writable keys grouped by their Proton settings page.
func settingsSetCmd(scope string, specs map[string]settingSpec) *cobra.Command {
	return &cobra.Command{
		Use:   "set [KEY VALUE]",
		Short: fmt.Sprintf("Update %s %s setting (run with no arguments to list keys)", article(scope), scope),
		Args:  cobra.MaximumNArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return settingKeys(specs), cobra.ShellCompDirectiveNoFileComp
			case 1:
				return specs[args[0]].completions(), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: run([]Step{stepAuth}, func(c *Invocation) error {
			if len(c.Args) == 0 {
				printSettingKeys(c, scope, specs)
				return nil
			}
			key := c.Args[0]
			spec, ok := specs[key]
			if !ok {
				return fmt.Errorf("unknown %s setting %q; run `%s` to list keys", scope, key, settingsSetPath(scope))
			}
			if len(c.Args) == 1 {
				return fmt.Errorf("%s needs a value; accepts %s", key, spec.allowed())
			}
			val, err := spec.parse(c.Args[1])
			if err != nil {
				return fmt.Errorf("%s %w", key, err)
			}
			if c.dryRun("set %s = %v", key, val) {
				return nil
			}
			if err := c.App.API.Decode(c.Ctx, proton.Request{
				Method: "PUT", Path: spec.Path, Body: map[string]any{spec.Field: val},
			}, nil); err != nil {
				return err
			}
			c.R().Success(fmt.Sprintf("Set %s = %v", key, val))
			return nil
		}),
	}
}

// settingsSetPath renders the command a user should run to list a scope's keys.
func settingsSetPath(scope string) string {
	if scope == "account" {
		return "proton-cli settings set"
	}
	return "proton-cli " + scope + " settings set"
}

// article picks the indefinite article for a scope name, so the generated help
// reads "an account setting" and "a mail setting".
func article(scope string) string {
	if scope == "" {
		return "a"
	}
	if strings.ContainsRune("aeiou", rune(scope[0])) {
		return "an"
	}
	return "a"
}

func settingKeys(specs map[string]settingSpec) []string {
	out := make([]string, 0, len(specs))
	for k := range specs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// printSettingKeys lists the writable keys grouped by settings page, so the
// listing maps onto the pages a user already knows from the web client.
func printSettingKeys(c *Invocation, scope string, specs map[string]settingSpec) {
	byPage := map[string][]string{}
	for _, k := range settingKeys(specs) {
		p := specs[k].Page
		byPage[p] = append(byPage[p], k)
	}
	pages := make([]string, 0, len(byPage))
	for p := range byPage {
		pages = append(pages, p)
	}
	sort.Strings(pages)

	width := 0
	for k := range specs {
		if len(k) > width {
			width = len(k)
		}
	}
	out := c.R().Stdout
	_, _ = fmt.Fprintf(out, "Available %s settings (%s KEY VALUE):\n", scope, settingsSetPath(scope))
	for _, p := range pages {
		_, _ = fmt.Fprintf(out, "\n  %s\n", c.R().Out.Hint(p))
		for _, k := range byPage[p] {
			spec := specs[k]
			_, _ = fmt.Fprintf(out, "    %-*s  %s\n", width, k, spec.allowed())
			if spec.Desc != "" && spec.Desc != spec.allowed() {
				_, _ = fmt.Fprintf(out, "    %-*s  %s\n", width, "", c.R().Out.Hint(spec.Desc))
			}
		}
	}
}

// ── settings (account level) ──

// accountSettingSpecs covers the account-level pages the CLI can write:
// "Language and time" and the privacy half of "Security and privacy". Password,
// two-factor, account deletion, recovery secrets and billing are deliberately
// absent - they are not things a script should be able to change in one line.
// Proton Sentinel and Dark Web Monitoring are absent too: Proton stores them by
// calling enable/disable endpoints rather than writing a value, and silently
// downgrading a security feature does not belong behind `set`.
var accountSettingSpecs = map[string]settingSpec{
	"crash-reports": {
		Path: "/core/v4/settings/crashreports", Field: "CrashReports",
		Page: "Security and privacy", Desc: "send crash reports to Proton", Enum: onOff(),
	},
	"date-format": {
		Path: "/core/v4/settings/dateformat", Field: "DateFormat",
		Page: "Language and time", Desc: "how dates are written",
		Enum: []enumValue{{"locale", 0}, {"dd/mm/yyyy", 1}, {"mm/dd/yyyy", 2}, {"yyyy-mm-dd", 3}},
	},
	"locale": {
		Path: "/core/v4/settings/locale", Field: "Locale",
		Page: "Language and time", Desc: "interface language, e.g. en_US or de_AT",
	},
	"telemetry": {
		Path: "/core/v4/settings/telemetry", Field: "Telemetry",
		Page: "Security and privacy", Desc: "send anonymous usage data to Proton", Enum: onOff(),
	},
	"time-format": {
		Path: "/core/v4/settings/timeformat", Field: "TimeFormat",
		Page: "Language and time", Desc: "clock format",
		Enum: []enumValue{{"locale", 0}, {"24h", 1}, {"12h", 2}},
	},
	"week-start": {
		Path: "/core/v4/settings/weekstart", Field: "WeekStart",
		Page: "Language and time", Desc: "first day of the week",
		Enum: []enumValue{{"locale", 0}, {"monday", 1}, {"tuesday", 2}, {"wednesday", 3},
			{"thursday", 4}, {"friday", 5}, {"saturday", 6}, {"sunday", 7}},
	},
}

func newSettingsCmd() *cobra.Command {
	return settingsCmd("account", "Show account settings", accountSettingSpecs, func(c *Invocation) error {
		resp, err := c.App.API.Do(c.Ctx, proton.Request{Method: "GET", Path: "/core/v4/settings"})
		if err != nil {
			return err
		}
		if c.R().Format != render.FormatText {
			return c.R().JSON(resp.Body)
		}
		return printSettingsText(c, resp.Body, renderAccountSettings)
	})
}

func printSettingsText(c *Invocation, body []byte, renderer func(*Invocation, map[string]any)) error {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return c.R().JSON(body)
	}
	renderer(c, m)
	return nil
}

func renderAccountSettings(c *Invocation, m map[string]any) {
	u, _ := m["UserSettings"].(map[string]any)
	if u == nil {
		_ = c.R().Object(m)
		return
	}
	p := fieldPrinter(c, 16)
	p("Locale", str(u["Locale"]))
	p("Date Format", enumName(accountSettingSpecs["date-format"], u["DateFormat"]))
	p("Time Format", enumName(accountSettingSpecs["time-format"], u["TimeFormat"]))
	p("Week Start", enumName(accountSettingSpecs["week-start"], u["WeekStart"]))
	if e, ok := u["Email"].(map[string]any); ok {
		p("Recovery Email", str(e["Value"]))
	}
	if ph, ok := u["Phone"].(map[string]any); ok {
		p("Recovery Phone", str(ph["Value"]))
	}
	p("Telemetry", onOffText(intOf(u["Telemetry"])))
	p("Crash Reports", onOffText(intOf(u["CrashReports"])))
	if hs, ok := u["HighSecurity"].(map[string]any); ok {
		p("High Security", onOffText(intOf(hs["Value"])))
	}
}

// ── shared rendering helpers ──

// fieldPrinter returns a "Label: value" printer with a fixed label column.
func fieldPrinter(c *Invocation, width int) func(label, value string) {
	return func(label, value string) {
		_, _ = fmt.Fprintf(c.R().Stdout, "%-*s %s\n", width+1, label+":", value)
	}
}

// enumName maps a raw API value back to the spec's human name, so reads and
// writes speak the same vocabulary. Unknown values render as the raw number.
func enumName(spec settingSpec, v any) string {
	n := intOf(v)
	for _, e := range spec.Enum {
		if e.N == n {
			return e.Name
		}
	}
	return strconv.Itoa(n)
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOf(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

func onOffText(i int) string {
	if i == 1 {
		return "on"
	}
	return "off"
}
