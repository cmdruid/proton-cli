package pass

import (
	"testing"

	pb "github.com/roman-16/proton-cli/internal/service/pass/proto"
)

func TestExtraFieldsParseTextAndHidden(t *testing.T) {
	fields, err := parseExtraFields(ExtraFields{
		Text:   []string{"Server=db01.example.com"},
		Hidden: []string{"Root Password=hunter2"},
	})
	if err != nil {
		t.Fatalf("parseExtraFields: %v", err)
	}
	loose, sections := split(fields)
	if len(sections) != 0 {
		t.Errorf("fields naming no section produced %d sections", len(sections))
	}
	if len(loose) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(loose))
	}
	text, ok := loose[0].Content.(*pb.ExtraField_Text)
	if loose[0].FieldName != "Server" || !ok || text.Text.Content != "db01.example.com" {
		t.Errorf("text field = %v", loose[0])
	}
	hidden, ok := loose[1].Content.(*pb.ExtraField_Hidden)
	if loose[1].FieldName != "Root Password" || !ok || hidden.Hidden.Content != "hunter2" {
		t.Errorf("hidden field = %v", loose[1])
	}
}

// A value may hold anything, including the characters that mean something in the
// part before it.
func TestExtraFieldsSplitOnTheFirstEqualsOnly(t *testing.T) {
	fields, err := parseExtraFields(ExtraFields{Text: []string{"Query=a=1&b=2"}})
	if err != nil {
		t.Fatalf("parseExtraFields: %v", err)
	}
	if got := fields[0].value; got != "a=1&b=2" {
		t.Errorf("value = %q, want a=1&b=2", got)
	}
}

func TestExtraFieldsRefuseWhatCannotBeRead(t *testing.T) {
	for _, raw := range []string{"no-equals-sign", "=orphan", "Section/=x", "/Name=x"} {
		if _, err := parseExtraFields(ExtraFields{Text: []string{raw}}); err == nil {
			t.Errorf("%q was accepted; it names no field", raw)
		}
	}
	if _, err := parseExtraFields(ExtraFields{Hidden: []string{"alsobad"}}); err == nil {
		t.Error("a hidden field without '=' was accepted")
	}
}

// Fields naming a section are grouped under it, in the order their sections
// first appeared, and fields naming none stay loose.
func TestFieldsAreGroupedByTheSectionTheyName(t *testing.T) {
	fields, err := parseExtraFields(ExtraFields{
		Text:   []string{"Loose=1", "Router/Address=192.168.0.1", "Recovery/Code=abc", "Router/Model=X"},
		Hidden: []string{"Recovery/Key=secret"},
	})
	if err != nil {
		t.Fatalf("parseExtraFields: %v", err)
	}
	loose, sections := split(fields)

	if len(loose) != 1 || loose[0].FieldName != "Loose" {
		t.Errorf("loose fields = %v, want just Loose", loose)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].SectionName != "Router" || sections[1].SectionName != "Recovery" {
		t.Errorf("sections came out as %q and %q; they keep the order they first appeared in",
			sections[0].SectionName, sections[1].SectionName)
	}
	if len(sections[0].SectionFields) != 2 {
		t.Errorf("Router holds %d fields, want 2", len(sections[0].SectionFields))
	}
	// A hidden field goes under its section like any other.
	if len(sections[1].SectionFields) != 2 {
		t.Errorf("Recovery holds %d fields, want 2", len(sections[1].SectionFields))
	}
	if _, ok := sections[1].SectionFields[1].Content.(*pb.ExtraField_Hidden); !ok {
		t.Error("the hidden field lost its hiddenness on the way into a section")
	}
}

// Two sections can hold a field of the same name, so a patch has to identify one
// by both.
func TestPatchingAFieldChangesOnlyTheOneItNames(t *testing.T) {
	it := &pb.Item{Content: &pb.Content{Content: &pb.Content_Custom{Custom: &pb.ItemCustom{}}}}
	if err := patchExtraFields(it, Patch{ExtraFields: ExtraFields{Text: []string{
		"Router/Password=one", "Wifi/Password=two", "Loose=three",
	}}}); err != nil {
		t.Fatalf("patchExtraFields: %v", err)
	}
	if err := patchExtraFields(it, Patch{ExtraFields: ExtraFields{Text: []string{"Router/Password=changed"}}}); err != nil {
		t.Fatalf("patchExtraFields: %v", err)
	}

	got := map[string]string{}
	for _, f := range it.ExtraFields {
		got[FieldRef("", f.FieldName)] = f.GetText().GetContent()
	}
	for _, s := range sectionsOf(it.Content) {
		for _, f := range s.GetSectionFields() {
			got[FieldRef(s.GetSectionName(), f.FieldName)] = f.GetText().GetContent()
		}
	}
	want := map[string]string{
		"Router/Password": "changed", "Wifi/Password": "two", "Loose": "three",
	}
	for ref, v := range want {
		if got[ref] != v {
			t.Errorf("%s = %q, want %q", ref, got[ref], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the item holds %d fields, want %d: %v", len(got), len(want), got)
	}
}

// Only the types whose editor offers headings can carry them, so a section given
// to any other is refused rather than dropped on the way in.
func TestOnlySomeItemsCanHoldASection(t *testing.T) {
	login := &pb.Item{Content: &pb.Content{Content: &pb.Content_Login{Login: &pb.ItemLogin{}}}}
	if err := patchExtraFields(login, Patch{ExtraFields: ExtraFields{Text: []string{"Recovery/Code=1"}}}); err == nil {
		t.Error("a login accepted a section; it has nowhere to store one")
	}
	// A field naming no section is fine on anything.
	if err := patchExtraFields(login, Patch{ExtraFields: ExtraFields{Text: []string{"Code=1"}}}); err != nil {
		t.Errorf("a login refused a field with no section: %v", err)
	}
	for _, kind := range []string{"custom", "ssh-key", "wifi", "identity"} {
		if !SectionsAllowed(kind) {
			t.Errorf("%s should be able to hold a section", kind)
		}
	}
	for _, kind := range []string{"login", "note", "credit-card", "alias"} {
		if SectionsAllowed(kind) {
			t.Errorf("%s should not be able to hold a section", kind)
		}
	}
}

// What a record shows is what --field accepts.
func TestAFieldReadsBackTheWayItIsWritten(t *testing.T) {
	for _, tt := range []struct{ section, name, want string }{
		{"", "Code", "Code"},
		{"Recovery", "Code", "Recovery/Code"},
	} {
		if got := (ItemField{Section: tt.section, Name: tt.name}).Ref(); got != tt.want {
			t.Errorf("Ref() = %q, want %q", got, tt.want)
		}
	}
}

// A two-factor field holds a secret Pass turns into a code, so a value nothing
// can read is refused rather than stored as a field that will never produce one.
func TestATwoFactorFieldMustHoldAReadableSecret(t *testing.T) {
	fields, err := parseExtraFields(ExtraFields{TOTP: []string{"Backup=JBSWY3DPEHPK3PXP"}})
	if err != nil {
		t.Fatalf("a bare base32 secret was refused: %v", err)
	}
	if _, ok := fields[0].pb().Content.(*pb.ExtraField_Totp); !ok {
		t.Error("the field was not stored as a two-factor one")
	}
	if _, err := parseExtraFields(ExtraFields{
		TOTP: []string{"Backup=otpauth://totp/x?secret=JBSWY3DPEHPK3PXP"},
	}); err != nil {
		t.Errorf("a TOTP URI was refused: %v", err)
	}
	// 0, 1, 8 and 9 are not in the base32 alphabet, so nothing can come out of it.
	for _, bad := range []string{"Backup=0189!!", "Backup="} {
		if _, err := parseExtraFields(ExtraFields{TOTP: []string{bad}}); err == nil {
			t.Errorf("%q was accepted; no code can come out of it", bad)
		}
	}
}

// The three kinds go under a section like any other field.
func TestEveryKindOfFieldCanSitInASection(t *testing.T) {
	fields, err := parseExtraFields(ExtraFields{
		Text:   []string{"Recovery/Note=keep"},
		Hidden: []string{"Recovery/Key=secret"},
		TOTP:   []string{"Recovery/Code=JBSWY3DPEHPK3PXP"},
	})
	if err != nil {
		t.Fatalf("parseExtraFields: %v", err)
	}
	loose, sections := split(fields)
	if len(loose) != 0 {
		t.Errorf("%d fields stayed loose", len(loose))
	}
	if len(sections) != 1 || len(sections[0].SectionFields) != 3 {
		t.Fatalf("expected one section of three fields, got %v", sections)
	}
}
