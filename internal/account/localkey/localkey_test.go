package localkey

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/cmdruid/proton-cli/internal/proton"
)

// fakeDoer captures the last request and replays a canned response body.
type fakeDoer struct {
	last     proton.Request
	respBody []byte
}

func (f *fakeDoer) Do(_ context.Context, r proton.Request) (*proton.Response, error) {
	f.last = r
	return &proton.Response{Status: 200, Body: f.respBody}, nil
}

func (f *fakeDoer) Decode(_ context.Context, r proton.Request, out any) error {
	f.last = r
	if out == nil || f.respBody == nil {
		return nil
	}
	return json.Unmarshal(f.respBody, out)
}

func TestWrapUnwrapRoundTrip(t *testing.T) {
	key, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	const skp = "the-salted-key-password-31-bytes"
	blob, err := Wrap(skp, key)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if blob == "" || blob == skp {
		t.Fatalf("blob should be encrypted, got %q", blob)
	}
	if _, err := base64.StdEncoding.DecodeString(blob); err != nil {
		t.Errorf("blob is not base64: %v", err)
	}
	got, err := Unwrap(blob, key)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if got != skp {
		t.Errorf("round-trip = %q, want %q", got, skp)
	}
}

func TestUnwrapWrongKeyFails(t *testing.T) {
	key, _ := Generate()
	other, _ := Generate()
	blob, err := Wrap("secret", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Unwrap(blob, other); err == nil {
		t.Error("Unwrap with a different client key should fail")
	}
}

func TestPutSendsBase64Key(t *testing.T) {
	f := &fakeDoer{}
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	if err := Put(context.Background(), f, key); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if f.last.Method != "PUT" || f.last.Path != localKeyPath {
		t.Errorf("Put issued %s %s, want PUT %s", f.last.Method, f.last.Path, localKeyPath)
	}
	body, ok := f.last.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is not map[string]any: %T", f.last.Body)
	}
	if got := body["Key"]; got != base64.StdEncoding.EncodeToString(key) {
		t.Errorf("Key = %v, want base64 of the key", got)
	}
}

func TestGetDecodesClientKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	f := &fakeDoer{respBody: []byte(`{"Code":1000,"ClientKey":"` + base64.StdEncoding.EncodeToString(key) + `"}`)}
	got, err := Get(context.Background(), f)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if f.last.Method != "GET" || f.last.Path != localKeyPath {
		t.Errorf("Get issued %s %s, want GET %s", f.last.Method, f.last.Path, localKeyPath)
	}
	if string(got) != string(key) {
		t.Errorf("client key = %q, want %q", got, key)
	}
}

func TestGetMissingClientKeyErrors(t *testing.T) {
	f := &fakeDoer{respBody: []byte(`{"Code":1000}`)}
	if _, err := Get(context.Background(), f); err == nil {
		t.Error("Get should error when the server returns no ClientKey")
	}
}
