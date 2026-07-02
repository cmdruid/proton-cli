package mail

import (
	"encoding/base64"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestBuildMIMEMessageStructure(t *testing.T) {
	body := "hello world"
	atts := []preparedAttachment{
		{Filename: "note.txt", MIMEType: "text/plain", Data: []byte("attached bytes")},
	}
	out, err := buildMIMEMessage(body, "text/plain", atts)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}

	if !strings.HasPrefix(out, "Content-Type: multipart/mixed; boundary=") {
		t.Errorf("missing top-level multipart header:\n%s", out)
	}
	if !strings.HasSuffix(out, "\r\n") {
		t.Error("MIME message must end with a trailing CRLF (PGP/MIME signature safety)")
	}

	// The whole thing must parse as a valid multipart entity with two parts,
	// and each part's base64 body must decode to the original bytes.
	mediaType, params, err := mime.ParseMediaType(topContentType(t, out))
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("media type = %q, want multipart/mixed", mediaType)
	}
	_, rest, _ := strings.Cut(out, "\r\n\r\n")
	mr := multipart.NewReader(strings.NewReader(rest), params["boundary"])

	bodyPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read body part: %v", err)
	}
	if cte := bodyPart.Header.Get("Content-Transfer-Encoding"); cte != "base64" {
		t.Errorf("body CTE = %q, want base64", cte)
	}
	if got := decodePartBase64(t, bodyPart); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}

	attPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read attachment part: %v", err)
	}
	if disp := attPart.Header.Get("Content-Disposition"); !strings.Contains(disp, `filename="note.txt"`) {
		t.Errorf("attachment disposition = %q, want filename note.txt", disp)
	}
	if got := decodePartBase64(t, attPart); got != "attached bytes" {
		t.Errorf("attachment = %q, want %q", got, "attached bytes")
	}

	if _, err := mr.NextPart(); err == nil {
		t.Error("expected exactly two parts")
	}
}

func TestBuildMIMEMessageNoAttachments(t *testing.T) {
	out, err := buildMIMEMessage("<b>hi</b>", "text/html", nil)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	if !strings.Contains(out, "text/html; charset=utf-8") {
		t.Errorf("expected html body part content-type, got:\n%s", out)
	}
}

// A part carrying a ContentID must be emitted inline (with a Content-ID header)
// rather than as a downloadable attachment, so embedded images render in place
// for external PGP/MIME recipients.
func TestBuildMIMEMessageInlineDisposition(t *testing.T) {
	out, err := buildMIMEMessage("<p>hi</p>", "text/html", []preparedAttachment{
		{Filename: "pic.png", MIMEType: "image/png", Data: []byte("PNGDATA"), ContentID: "abc123@proton.me"},
	})
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	_, params, err := mime.ParseMediaType(topContentType(t, out))
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}
	_, rest, _ := strings.Cut(out, "\r\n\r\n")
	mr := multipart.NewReader(strings.NewReader(rest), params["boundary"])

	if _, err := mr.NextPart(); err != nil {
		t.Fatalf("read body part: %v", err)
	}
	inline, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read inline part: %v", err)
	}
	if disp := inline.Header.Get("Content-Disposition"); !strings.HasPrefix(disp, "inline") {
		t.Errorf("Content-Disposition = %q, want it to start with inline", disp)
	}
	if cid := inline.Header.Get("Content-Id"); cid != "<abc123@proton.me>" {
		t.Errorf("Content-ID = %q, want <abc123@proton.me>", cid)
	}
}

func TestWrapBase64BreaksAt76(t *testing.T) {
	data := make([]byte, 200)
	wrapped := string(wrapBase64(data))
	for _, line := range strings.Split(wrapped, "\r\n") {
		if len(line) > 76 {
			t.Fatalf("line exceeds 76 chars: %d", len(line))
		}
	}
	// It must still decode back to the original once unwrapped.
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(wrapped, "\r\n", ""))
	if err != nil {
		t.Fatalf("unwrapped base64 does not decode: %v", err)
	}
	if len(decoded) != len(data) {
		t.Errorf("decoded %d bytes, want %d", len(decoded), len(data))
	}
}

func topContentType(t *testing.T, mimeMsg string) string {
	t.Helper()
	head, _, ok := strings.Cut(mimeMsg, "\r\n\r\n")
	if !ok {
		t.Fatalf("no header/body separator in MIME message")
	}
	return strings.TrimPrefix(head, "Content-Type: ")
}

func decodePartBase64(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	buf := make([]byte, 4096)
	var raw strings.Builder
	for {
		n, err := r.Read(buf)
		raw.Write(buf[:n])
		if err != nil {
			break
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.ReplaceAll(raw.String(), "\r\n", ""), "\n", ""))
	if err != nil {
		t.Fatalf("part base64 decode: %v", err)
	}
	return string(decoded)
}
