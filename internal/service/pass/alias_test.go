package pass

import (
	"strings"
	"testing"
)

// Proton mints the word in front of a suffix afresh on every request, so the
// only part of one that lasts long enough to be typed back is the domain. A
// whole suffix read a moment ago is a value that has already stopped working,
// and saying so beats quietly picking a different address than was asked for.
func TestASuffixIsChosenByItsDomain(t *testing.T) {
	offered := []AliasSuffix{
		{Suffix: ".voice819@passinbox.com", Domain: "passinbox.com"},
		{Suffix: ".germicide770@passfwd.com", Domain: "passfwd.com"},
	}
	for _, tc := range []struct {
		name, wanted, want string
	}{
		{"a domain", "passfwd.com", ".germicide770@passfwd.com"},
		{"a domain with its at sign", "@passinbox.com", ".voice819@passinbox.com"},
		{"the whole suffix, still current", ".voice819@passinbox.com", ".voice819@passinbox.com"},
		{"nothing at all", "", ".voice819@passinbox.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickSuffix(offered, tc.wanted)
			if err != nil {
				t.Fatalf("pickSuffix(%q): %v", tc.wanted, err)
			}
			if got.Suffix != tc.want {
				t.Errorf("pickSuffix(%q) = %q, want %q", tc.wanted, got.Suffix, tc.want)
			}
		})
	}
}

// A suffix from a moment ago names a word Proton has already moved on from, so
// the refusal has to name the domains rather than repeat the mistake.
func TestAStaleSuffixIsRefusedByNamingTheDomains(t *testing.T) {
	offered := []AliasSuffix{
		{Suffix: ".voice819@passinbox.com", Domain: "passinbox.com"},
		{Suffix: ".germicide770@passfwd.com", Domain: "passfwd.com"},
	}
	_, err := pickSuffix(offered, ".undone942@passinbox.com")
	if err == nil {
		t.Fatal("a suffix Proton has moved on from was accepted")
	}
	for _, want := range []string{"passinbox.com", "passfwd.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), ".voice819@") {
		t.Errorf("the refusal offers another value that will not last: %q", err)
	}
}
