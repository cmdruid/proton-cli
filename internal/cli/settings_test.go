package cli

import (
	"strings"
	"testing"
)

func TestSettingSpecParseEnumAcceptsNamesAndNumbers(t *testing.T) {
	spec := mailSettingSpecs["view-mode"]
	for _, in := range []string{"conversations", "CONVERSATIONS", "0"} {
		got, err := spec.parse(in)
		if err != nil {
			t.Fatalf("parse(%q): %v", in, err)
		}
		if got != 0 {
			t.Errorf("parse(%q) = %v, want 0", in, got)
		}
	}
	if got, err := spec.parse("messages"); err != nil || got != 1 {
		t.Errorf("parse(messages) = %v, %v", got, err)
	}
}

func TestSettingSpecParseEnumRejectsOutOfDomainValues(t *testing.T) {
	spec := mailSettingSpecs["view-mode"]
	for _, in := range []string{"7", "threads", ""} {
		_, err := spec.parse(in)
		if err == nil {
			t.Errorf("parse(%q) should have failed", in)
			continue
		}
		// The error names the whole domain, so the user never has to guess.
		if !strings.Contains(err.Error(), "conversations, messages") {
			t.Errorf("parse(%q) error = %q, want it to list the allowed values", in, err)
		}
	}
}

func TestSettingSpecParseRangeIsBounded(t *testing.T) {
	spec := mailSettingSpecs["delay-send"]
	for _, in := range []string{"0", "20", "7"} {
		if _, err := spec.parse(in); err != nil {
			t.Errorf("parse(%q): %v", in, err)
		}
	}
	for _, in := range []string{"-1", "21", "999", "soon"} {
		err := mustFail(t, spec, in)
		if !strings.Contains(err.Error(), "0-20 (seconds)") {
			t.Errorf("parse(%q) error = %q, want it to state the range and unit", in, err)
		}
	}
}

func TestSettingSpecParseTextChoices(t *testing.T) {
	spec := mailSettingSpecs["draft-type"]
	got, err := spec.parse("TEXT/HTML")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The canonical spelling is stored, not what the user typed.
	if got != "text/html" {
		t.Errorf("parse = %v, want text/html", got)
	}
	if err := mustFail(t, spec, "text/markdown"); !strings.Contains(err.Error(), "text/html, text/plain") {
		t.Errorf("error = %q", err)
	}
}

func TestSettingSpecParseFreeTextRejectsOnlyEmpty(t *testing.T) {
	spec := accountSettingSpecs["locale"]
	if _, err := spec.parse("de_AT"); err != nil {
		t.Errorf("parse: %v", err)
	}
	_ = mustFail(t, spec, "")
}

func TestSettingSpecAllowedRendersEachDomain(t *testing.T) {
	tests := map[string]string{
		"page-size":  "50, 100, 200",
		"delay-send": "0-20 (seconds)",
		"draft-type": "text/html, text/plain",
		"shortcuts":  "off, on",
	}
	for key, want := range tests {
		if got := mailSettingSpecs[key].allowed(); got != want {
			t.Errorf("%s allowed() = %q, want %q", key, got, want)
		}
	}
	if got := accountSettingSpecs["locale"].allowed(); got == "" {
		t.Error("a free-text setting should still describe itself")
	}
}

func TestSettingSpecCompletionsOfferFiniteDomainsOnly(t *testing.T) {
	if got := mailSettingSpecs["view-mode"].completions(); strings.Join(got, ",") != "conversations,messages" {
		t.Errorf("enum completions = %v", got)
	}
	if got := mailSettingSpecs["draft-type"].completions(); len(got) != 2 {
		t.Errorf("text completions = %v", got)
	}
	if got := mailSettingSpecs["delay-send"].completions(); got != nil {
		t.Errorf("a range has no finite completions, got %v", got)
	}
	if got := accountSettingSpecs["locale"].completions(); got != nil {
		t.Errorf("free text has no finite completions, got %v", got)
	}
}

// Every declared setting must be usable: an endpoint, a field, a page to group
// it under, and a value domain a user can discover.
func TestEverySettingSpecIsComplete(t *testing.T) {
	tables := map[string]map[string]settingSpec{
		"account":  accountSettingSpecs,
		"mail":     mailSettingSpecs,
		"calendar": calendarSettingSpecs,
		"drive":    driveSettingSpecs,
	}
	for scope, specs := range tables {
		for key, spec := range specs {
			if spec.Path == "" || spec.Field == "" {
				t.Errorf("%s.%s has no endpoint or field", scope, key)
			}
			if spec.Page == "" {
				t.Errorf("%s.%s has no settings page to group it under", scope, key)
			}
			if spec.Desc == "" {
				t.Errorf("%s.%s has no description", scope, key)
			}
			if key != strings.ToLower(key) || strings.Contains(key, "_") {
				t.Errorf("%s.%s should be lower-case and hyphenated", scope, key)
			}
			domains := 0
			for _, set := range []bool{len(spec.Enum) > 0, spec.Range != nil, len(spec.Text) > 0} {
				if set {
					domains++
				}
			}
			if domains > 1 {
				t.Errorf("%s.%s declares more than one value domain", scope, key)
			}
		}
	}
}

func TestEnumNameMapsAPIValuesBackToNames(t *testing.T) {
	spec := mailSettingSpecs["view-mode"]
	if got := enumName(spec, float64(0)); got != "conversations" {
		t.Errorf("enumName(0) = %q", got)
	}
	if got := enumName(spec, float64(1)); got != "messages" {
		t.Errorf("enumName(1) = %q", got)
	}
	// An unknown value renders as itself rather than silently becoming a name.
	if got := enumName(spec, float64(9)); got != "9" {
		t.Errorf("enumName(9) = %q, want the raw number", got)
	}
}

func TestArticleAgreesWithTheScopeName(t *testing.T) {
	for scope, want := range map[string]string{
		"account": "an", "mail": "a", "calendar": "a", "drive": "a", "": "a",
	} {
		if got := article(scope); got != want {
			t.Errorf("article(%q) = %q, want %q", scope, got, want)
		}
	}
}

func TestSettingsSetPathNamesTheCommandToRun(t *testing.T) {
	if got := settingsSetPath("account"); got != "proton-cli settings set" {
		t.Errorf("account path = %q", got)
	}
	if got := settingsSetPath("mail"); got != "proton-cli mail settings set" {
		t.Errorf("mail path = %q", got)
	}
}

func mustFail(t *testing.T, spec settingSpec, in string) error {
	t.Helper()
	_, err := spec.parse(in)
	if err == nil {
		t.Fatalf("parse(%q) should have failed", in)
	}
	return err
}
