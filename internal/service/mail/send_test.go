package mail

import (
	"reflect"
	"testing"
)

func TestDedupeRecipientsIsCaseInsensitiveAndPreservesFirstSeenOrder(t *testing.T) {
	got := dedupeRecipients(
		[]string{"alice@proton.me", "Bob@Example.com"},
		[]string{"alice@PROTON.me", "carol@proton.me"}, // alice is a case-insensitive dup
		[]string{"bob@example.com"},                    // bob is a dup
	)
	want := []string{"alice@proton.me", "Bob@Example.com", "carol@proton.me"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dedupeRecipients = %v, want %v", got, want)
	}
}

func TestRecipientListPairsAddressAndName(t *testing.T) {
	got := recipientList([]string{"jane@proton.me"})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0]["Address"] != "jane@proton.me" || got[0]["Name"] != "jane@proton.me" {
		t.Errorf("recipientList entry = %v", got[0])
	}
	if recipientList(nil) == nil {
		t.Error("recipientList(nil) should return a non-nil empty slice for JSON encoding")
	}
}
