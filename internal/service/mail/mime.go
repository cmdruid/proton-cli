package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/textproto"
)

// buildMIMEMessage assembles a multipart/mixed MIME entity (top-level headers +
// parts) with the message body followed by any attachments, all base64-encoded.
// The returned string is the complete MIME entity that a PGP/MIME send encrypts
// as one unit.
//
// The trailing CRLF is deliberate: without it some clients (Outlook) append one
// and break the PGP/MIME signature, matching the web client's constructMime.
func buildMIMEMessage(body, mimeType string, atts []preparedAttachment) (string, error) {
	if mimeType == "" {
		mimeType = "text/plain"
	}

	var parts bytes.Buffer
	w := multipart.NewWriter(&parts)

	bodyHeader := textproto.MIMEHeader{}
	bodyHeader.Set("Content-Type", mimeType+"; charset=utf-8")
	bodyHeader.Set("Content-Transfer-Encoding", "base64")
	bodyPart, err := w.CreatePart(bodyHeader)
	if err != nil {
		return "", err
	}
	if _, err := bodyPart.Write(wrapBase64([]byte(body))); err != nil {
		return "", err
	}

	for _, a := range atts {
		ct := a.MIMEType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", fmt.Sprintf("%s; name=%q", ct, a.Filename))
		h.Set("Content-Transfer-Encoding", "base64")
		h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Filename))
		p, err := w.CreatePart(h)
		if err != nil {
			return "", err
		}
		if _, err := p.Write(wrapBase64(a.Data)); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", w.Boundary())
	out.Write(parts.Bytes())
	out.WriteString("\r\n")
	return out.String(), nil
}

// wrapBase64 encodes data as base64 with CRLF breaks every 76 characters, per
// MIME line-length conventions.
func wrapBase64(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	var b bytes.Buffer
	for len(encoded) > lineLen {
		b.WriteString(encoded[:lineLen])
		b.WriteString("\r\n")
		encoded = encoded[lineLen:]
	}
	b.WriteString(encoded)
	return b.Bytes()
}
