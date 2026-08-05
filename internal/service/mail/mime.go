package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
)

// mimePart is one attachment rendered into a MIME entity. A non-empty ContentID
// marks it inline, so it belongs inside the multipart/related that wraps the
// HTML body rather than beside it.
type mimePart struct {
	Filename  string
	MIMEType  string
	Data      []byte
	ContentID string
}

// mimeEntity is a node of a MIME tree: a leaf carrying content, or a container
// carrying children. Assembling a tree and rendering it once keeps the two
// shapes the CLI needs - the flat multipart/mixed a PGP/MIME send encrypts, and
// the alternative/related nesting an exported .eml carries - expressible in the
// same code.
type mimeEntity struct {
	contentType  string
	encoding     string
	content      []byte
	extraHeaders [][2]string

	subtype  string
	children []mimeEntity
}

func (e mimeEntity) isContainer() bool { return e.subtype != "" }

// renderEntity returns the entity's own headers and its body separately, so a caller
// can either emit them together or splice the headers into an existing header
// block, which is what exporting a message does.
func renderEntity(e mimeEntity) (textproto.MIMEHeader, []byte, error) {
	h := textproto.MIMEHeader{}
	if !e.isContainer() {
		h.Set("Content-Type", e.contentType)
		if e.encoding != "" {
			h.Set("Content-Transfer-Encoding", e.encoding)
		}
		for _, kv := range e.extraHeaders {
			h.Set(kv[0], kv[1])
		}
		return h, e.content, nil
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, child := range e.children {
		ch, cb, err := renderEntity(child)
		if err != nil {
			return nil, nil, err
		}
		p, err := w.CreatePart(ch)
		if err != nil {
			return nil, nil, err
		}
		if _, err := p.Write(cb); err != nil {
			return nil, nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, nil, err
	}
	h.Set("Content-Type", fmt.Sprintf("multipart/%s; boundary=%q", e.subtype, w.Boundary()))
	for _, kv := range e.extraHeaders {
		h.Set(kv[0], kv[1])
	}
	return h, buf.Bytes(), nil
}

func textEntity(mimeType, content string) mimeEntity {
	if mimeType == "" {
		mimeType = mimeTypePlain
	}
	return mimeEntity{
		contentType: mimeType + "; charset=utf-8",
		encoding:    "base64",
		content:     wrapBase64([]byte(content)),
	}
}

func attachmentEntity(a mimePart) mimeEntity {
	ct := a.MIMEType
	if ct == "" {
		ct = "application/octet-stream"
	}
	e := mimeEntity{
		contentType: fmt.Sprintf("%s; name=%q", ct, a.Filename),
		encoding:    "base64",
		content:     wrapBase64(a.Data),
	}
	if a.ContentID != "" {
		e.extraHeaders = [][2]string{
			{"Content-Disposition", fmt.Sprintf("inline; filename=%q", a.Filename)},
			{"Content-ID", "<" + a.ContentID + ">"},
		}
	} else {
		e.extraHeaders = [][2]string{
			{"Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Filename)},
		}
	}
	return e
}

func attachmentEntities(atts []mimePart) []mimeEntity {
	out := make([]mimeEntity, 0, len(atts))
	for _, a := range atts {
		out = append(out, attachmentEntity(a))
	}
	return out
}

// buildMIMEMessage assembles the multipart/mixed entity a PGP/MIME send encrypts
// as one unit: the body followed by every attachment.
//
// The trailing CRLF is deliberate: without it some clients (Outlook) append one
// and break the PGP/MIME signature, matching the web client's constructMime.
func buildMIMEMessage(body, mimeType string, atts []mimePart) (string, error) {
	root := mimeEntity{
		subtype:  "mixed",
		children: append([]mimeEntity{textEntity(mimeType, body)}, attachmentEntities(atts)...),
	}
	h, b, err := renderEntity(root)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	out.WriteString(formatHeaders(h))
	out.WriteString("\r\n")
	out.Write(b)
	out.WriteString("\r\n")
	return out.String(), nil
}

// buildExportMIME assembles the entity an exported message carries, mirroring
// how the web client rebuilds one:
//
//	multipart/mixed
//	  multipart/alternative        both a text and an HTML body
//	    text/plain
//	    multipart/related          the HTML body plus its inline images
//	      text/html
//	      inline parts
//	  attachments
//
// Each layer collapses when it holds a single child, so a plain message with no
// attachments exports as a bare text/plain entity. Headers come back separately
// so the caller can merge them into the message's original header block.
func buildExportMIME(plain, html string, atts []mimePart) (textproto.MIMEHeader, []byte, error) {
	var inline, regular []mimePart
	for _, a := range atts {
		if a.ContentID != "" {
			inline = append(inline, a)
		} else {
			regular = append(regular, a)
		}
	}

	var bodyEntity mimeEntity
	switch {
	case html != "" && plain != "":
		bodyEntity = mimeEntity{subtype: "alternative", children: []mimeEntity{
			textEntity(mimeTypePlain, plain), htmlEntity(html, inline),
		}}
	case html != "":
		bodyEntity = htmlEntity(html, inline)
	default:
		bodyEntity = textEntity(mimeTypePlain, plain)
	}

	root := bodyEntity
	if len(regular) > 0 {
		root = mimeEntity{subtype: "mixed", children: append([]mimeEntity{bodyEntity}, attachmentEntities(regular)...)}
	}
	return renderEntity(root)
}

// htmlEntity wraps an HTML body with its inline images when there are any, so a
// cid: reference in the body resolves.
func htmlEntity(html string, inline []mimePart) mimeEntity {
	body := textEntity(mimeTypeHTML, html)
	if len(inline) == 0 {
		return body
	}
	return mimeEntity{subtype: "related", children: append([]mimeEntity{body}, attachmentEntities(inline)...)}
}

// formatHeaders renders a header block in a stable order, so identical input
// always produces identical output.
func formatHeaders(h textproto.MIMEHeader) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	return b.String()
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
