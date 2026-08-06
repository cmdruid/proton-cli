package pass

import (
	"testing"

	pb "github.com/roman-16/proton-cli/internal/service/pass/proto"
)

func TestBuildExtraFieldsParsesTextAndHidden(t *testing.T) {
	out, err := buildExtraFields(
		[]string{"Server=db01.example.com"},
		[]string{"Root Password=hunter2"},
	)
	if err != nil {
		t.Fatalf("buildExtraFields error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(out))
	}

	if out[0].FieldName != "Server" {
		t.Errorf("text field name = %q, want Server", out[0].FieldName)
	}
	text, ok := out[0].Content.(*pb.ExtraField_Text)
	if !ok || text.Text.Content != "db01.example.com" {
		t.Errorf("text field content = %v, want db01.example.com", out[0].Content)
	}

	if out[1].FieldName != "Root Password" {
		t.Errorf("hidden field name = %q, want 'Root Password'", out[1].FieldName)
	}
	hidden, ok := out[1].Content.(*pb.ExtraField_Hidden)
	if !ok || hidden.Hidden.Content != "hunter2" {
		t.Errorf("hidden field content = %v, want hunter2", out[1].Content)
	}
}

func TestBuildExtraFieldsRejectsMissingEquals(t *testing.T) {
	if _, err := buildExtraFields([]string{"no-equals-sign"}, nil); err == nil {
		t.Error("expected an error for a field without '='")
	}
	if _, err := buildExtraFields(nil, []string{"alsobad"}); err == nil {
		t.Error("expected an error for a hidden field without '='")
	}
}

func TestExtraFieldToItemDecodesEachContentType(t *testing.T) {
	cases := []struct {
		name              string
		field             *pb.ExtraField
		wantType, wantVal string
	}{
		{"text", &pb.ExtraField{FieldName: "a", Content: &pb.ExtraField_Text{Text: &pb.ExtraTextField{Content: "v1"}}}, "text", "v1"},
		{"hidden", &pb.ExtraField{FieldName: "b", Content: &pb.ExtraField_Hidden{Hidden: &pb.ExtraHiddenField{Content: "v2"}}}, "hidden", "v2"},
		{"totp", &pb.ExtraField{FieldName: "c", Content: &pb.ExtraField_Totp{Totp: &pb.ExtraTotp{TotpUri: "otpauth://totp/x"}}}, "totp", "otpauth://totp/x"},
	}
	for _, c := range cases {
		got := extraFieldToItem(c.field)
		if got.Name != c.field.FieldName {
			t.Errorf("%s: name = %q, want %q", c.name, got.Name, c.field.FieldName)
		}
		if got.Type != c.wantType {
			t.Errorf("%s: type = %q, want %q", c.name, got.Type, c.wantType)
		}
		if got.Value != c.wantVal {
			t.Errorf("%s: value = %q, want %q", c.name, got.Value, c.wantVal)
		}
	}
}
