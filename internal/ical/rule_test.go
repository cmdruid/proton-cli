package ical

import "testing"

// A rule is edited as text so that reading and re-saving an event cannot rewrite
// a rule somebody else authored.
func TestRuleWithPreservesEverythingElseVerbatim(t *testing.T) {
	const rule = "FREQ=WEEKLY;WKST=SU;BYDAY=MO,TH;INTERVAL=2"
	if got := RuleWith(rule, "COUNT", "5"); got != rule+";COUNT=5" {
		t.Errorf("RuleWith = %q", got)
	}
	if got := RuleWith(rule+";COUNT=9", "COUNT", "5"); got != rule+";COUNT=5" {
		t.Errorf("RuleWith did not replace in place: %q", got)
	}
}

func TestRuleValueIsCaseInsensitive(t *testing.T) {
	if got := RuleValue("freq=weekly;count=3", "COUNT"); got != "3" {
		t.Errorf("RuleValue = %q", got)
	}
	if got := RuleValue("FREQ=WEEKLY", "COUNT"); got != "" {
		t.Errorf("RuleValue on an absent part = %q", got)
	}
}

func TestRuleWithoutRemovesOnlyWhatItIsAsked(t *testing.T) {
	got := RuleWithout("FREQ=WEEKLY;COUNT=3;UNTIL=20261231;INTERVAL=2", "COUNT", "UNTIL")
	if got != "FREQ=WEEKLY;INTERVAL=2" {
		t.Errorf("RuleWithout = %q", got)
	}
}

func TestRuleCountIgnoresANonNumericValue(t *testing.T) {
	if n := ruleCount("FREQ=WEEKLY;COUNT=oops"); n != 0 {
		t.Errorf("ruleCount = %d, want 0 for a value that is not a number", n)
	}
	if n := ruleCount("FREQ=WEEKLY;COUNT=12"); n != 12 {
		t.Errorf("ruleCount = %d", n)
	}
}
