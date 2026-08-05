package mail

import (
	"context"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

// Proton send-package types (PACKAGE_TYPE).
const (
	pkgInternal  = 1  // SEND_PM: E2EE to a Proton user
	pkgEO        = 2  // SEND_EO: encrypted-for-outside (password link)
	pkgClear     = 4  // SEND_CLEAR: cleartext (TLS only)
	pkgPGPInline = 8  // SEND_PGP_INLINE: PGP-Inline (plaintext body) to an external key
	pkgPGPMIME   = 16 // SEND_PGP_MIME: encrypted to an external recipient's PGP key
)

const (
	// keyFlagEmailNoEncrypt (KEY_FLAG.FLAG_EMAIL_NO_ENCRYPT) marks a key that
	// cannot be used to encrypt mail (external address, e2ee-disabled, etc.).
	keyFlagEmailNoEncrypt = 4
	// apiKeySourceProton (API_KEY_SOURCE.PROTON) marks an internal Proton key;
	// any other source (WKD, KOO) is an external key.
	apiKeySourceProton = 0

	// defaultEOExpirationSeconds mirrors DEFAULT_EO_EXPIRATION_DAYS (28 days):
	// Proton always attaches an expiration to encrypted-for-outside messages.
	defaultEOExpirationSeconds = 28 * 24 * 60 * 60
	// srpAuthVersion is Proton's current SRP verifier version.
	srpAuthVersion = 4
)

// sendScheme is how a single recipient's copy is packaged.
type sendScheme int

const (
	schemeInternal       sendScheme = iota // Proton user -> E2EE
	schemeExternalPGP                      // external user with a usable PGP key -> PGP/MIME
	schemeExternalInline                   // external user with a pinned key preferring PGP-Inline
	schemeEO                               // external user + EO password -> password link
	schemeClear                            // external user, no key, no password -> cleartext
)

// externalScheme maps a pinned contact's x-pm-scheme to the send scheme for an
// external recipient: PGP-Inline when requested, otherwise PGP/MIME.
func externalScheme(pinScheme string) sendScheme {
	if pinScheme == "pgp-inline" {
		return schemeExternalInline
	}
	return schemeExternalPGP
}

// SendOptions describes a message to send. To/CC/BCC may each carry multiple
// recipients; at least one across the three is required. HTML switches the body
// MIME type to text/html.
type SendOptions struct {
	To      []string
	CC      []string
	BCC     []string
	Subject string
	Body    string
	HTML    bool
	// Attachments are local file paths to encrypt and attach.
	Attachments []string
	// InlineAttach are local image file paths embedded in the HTML body via a
	// generated Content-ID (disposition "inline"); requires HTML.
	InlineAttach []string
	// InlineAttachments are in-memory attachments (e.g. a generated ICS).
	InlineAttachments []InlineAttachment
	// DeliveryTime, when > 0, schedules the send for that absolute Unix time.
	DeliveryTime int64
	// ExpiresInSeconds, when > 0, makes the message self-destruct after that
	// many seconds. EO sends default to 28 days when this is 0.
	ExpiresInSeconds int
	// EOPassword, when set, password-protects the message for external
	// recipients (Encrypted Outside) instead of sending them cleartext.
	EOPassword string
	// EOPasswordHint is an optional hint shown to EO recipients.
	EOPasswordHint string
	// PinnedKeys carries contact-pinned encryption preferences per recipient
	// email (as it appears in To/CC/BCC), resolved by the caller from Contacts.
	// A recipient with a pinned key is encrypted to that key (see the web
	// client's encryption-preferences flow); absent means "no pins".
	PinnedKeys map[string]*PinnedRecipient
}

// PinnedRecipient is a recipient's contact-pinned encryption preferences,
// resolved from Contacts. Presence of a pinned key defaults encryption ON
// (unless Encrypt is explicitly false), matching the Proton web client.
type PinnedRecipient struct {
	ArmoredKeys       []string
	Encrypt           *bool
	Sign              *bool
	Scheme            string
	SignatureVerified bool
}

type apiPublicKey struct {
	PublicKey string
	Flags     int
	Source    int
}

type keysAllResponse struct {
	Address    struct{ Keys []apiPublicKey }
	Unverified struct{ Keys []apiPublicKey }
	ProtonMX   bool
}

type plannedRecipient struct {
	email      string
	scheme     sendScheme
	armoredKey string // recipient public key for internal + external-PGP schemes
}

func mailCapable(flags int) bool { return flags&keyFlagEmailNoEncrypt == 0 }

// classifyRecipient picks the send scheme (and recipient key, if any) from a
// /core/v4/keys/all response, mirroring the web client's getPublicKeys logic:
// mail-capable internal address keys mark a Proton user; otherwise a mail-capable
// external key (WKD/KOO) enables PGP/MIME; otherwise EO (with a password) or
// cleartext.
func classifyRecipient(resp keysAllResponse, eoPassword string) (sendScheme, string) {
	for _, k := range resp.Address.Keys {
		if mailCapable(k.Flags) {
			return schemeInternal, k.PublicKey
		}
	}
	for _, k := range resp.Unverified.Keys {
		if k.Source == apiKeySourceProton && mailCapable(k.Flags) {
			return schemeInternal, k.PublicKey
		}
	}
	for _, k := range resp.Unverified.Keys {
		if k.Source != apiKeySourceProton && mailCapable(k.Flags) {
			return schemeExternalPGP, k.PublicKey
		}
	}
	if eoPassword != "" {
		return schemeEO, ""
	}
	return schemeClear, ""
}

func (s *Service) planRecipient(ctx context.Context, email, eoPassword string, pin *PinnedRecipient) (plannedRecipient, error) {
	var resp keysAllResponse
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/keys/all",
		Query: proton.Query("Email", email, "InternalOnly", "0"),
	}, &resp); err != nil {
		return plannedRecipient{}, err
	}
	scheme, armored := classifyRecipient(resp, eoPassword)
	if pin != nil && len(pin.ArmoredKeys) > 0 && pinEncrypts(pin) {
		return planPinnedRecipient(email, scheme, armored, pin)
	}
	return plannedRecipient{email: email, scheme: scheme, armoredKey: armored}, nil
}

// pinEncrypts reports whether the pinned config asks us to encrypt. Presence of
// a pinned key defaults encryption ON (matching the web client); an explicit
// x-pm-encrypt:false opts out.
func pinEncrypts(pin *PinnedRecipient) bool {
	return pin.Encrypt == nil || *pin.Encrypt
}

// validForSending mirrors the web client's getIsValidForSending: a key must be
// encryption-capable and neither expired nor revoked.
func validForSending(key *pgp.Key) bool {
	return key.CanEncrypt() && !key.IsExpired() && !key.IsRevoked()
}

// planPinnedRecipient resolves a recipient's send scheme when their contact
// pins a key, mirroring extractEncryptionPreferences:
//   - internal / external-WKD: the recipient's primary API key must itself be
//     pinned (same fingerprint); we then send to the pinned copy. A mismatch is
//     the web client's PRIMARY_NOT_PINNED error.
//   - external without a server/WKD key: encrypt (PGP/MIME) to the first valid
//     pinned key.
func planPinnedRecipient(email string, base sendScheme, apiArmored string, pin *PinnedRecipient) (plannedRecipient, error) {
	if !pin.SignatureVerified {
		return plannedRecipient{}, fmt.Errorf(
			"contact signature for %s could not be verified; refusing to encrypt to an unverified pinned key", email)
	}
	type pinnedKey struct{ armored, fingerprint string }
	var valid []pinnedKey
	for _, a := range pin.ArmoredKeys {
		key, err := pgp.NewKeyFromArmored(a)
		if err != nil {
			continue
		}
		if !validForSending(key) {
			continue
		}
		valid = append(valid, pinnedKey{armored: a, fingerprint: key.GetFingerprint()})
	}
	if len(valid) == 0 {
		return plannedRecipient{}, fmt.Errorf(
			"no valid pinned key for %s (keys are expired, revoked, or not encryption-capable)", email)
	}
	switch base {
	case schemeInternal, schemeExternalPGP:
		primaryFingerprint := ""
		if apiArmored != "" {
			if k, err := pgp.NewKeyFromArmored(apiArmored); err == nil {
				primaryFingerprint = k.GetFingerprint()
			}
		}
		sendScheme := base
		if base == schemeExternalPGP {
			// A WKD external recipient may prefer PGP-Inline over PGP/MIME.
			sendScheme = externalScheme(pin.Scheme)
		}
		for _, v := range valid {
			if v.fingerprint == primaryFingerprint {
				return plannedRecipient{email: email, scheme: sendScheme, armoredKey: v.armored}, nil
			}
		}
		return plannedRecipient{}, fmt.Errorf(
			"the pinned key(s) for %s do not match the recipient's current primary key; "+
				"update the pinned key before sending", email)
	default:
		// External recipient with no server/WKD key: encrypt to the pinned key.
		return plannedRecipient{email: email, scheme: externalScheme(pin.Scheme), armoredKey: valid[0].armored}, nil
	}
}

// Send classifies each recipient and packages the message per scheme: internal
// (E2EE), external-PGP (PGP/MIME), encrypted-for-outside (password link), or
// cleartext. Internal/EO/cleartext recipients share one symmetric body; PGP/MIME
// recipients get a separately encrypted MIME body with embedded attachments.
// newDraftBody builds the POST /mail/v4/messages payload. The sender carries
// the address's display name, so recipients see "Jane Roe" rather than a bare
// address - the same pairing the web client sends.
func newDraftBody(sender keys.Address, opts SendOptions, armoredBody, mimeType string) map[string]any {
	return map[string]any{
		"Message": map[string]any{
			"ToList":   recipientList(opts.To),
			"CCList":   recipientList(opts.CC),
			"BCCList":  recipientList(opts.BCC),
			"Subject":  opts.Subject,
			"Sender":   map[string]string{"Address": sender.Email, "Name": sender.DisplayName},
			"Body":     armoredBody,
			"MIMEType": mimeType,
		},
	}
}

func (s *Service) Send(ctx context.Context, u *keys.Unlocked, opts SendOptions) (string, error) {
	addrKR, senderAddr, err := u.FirstAddr()
	if err != nil {
		return "", err
	}

	mimeType := "text/plain"
	if opts.HTML {
		mimeType = "text/html"
	}

	// Inline images assign a Content-ID and append a cid: reference to the body,
	// so this must happen before the draft body is encrypted below.
	inlinePrepared, err := prepareInlineImages(&opts, senderAddr.Email)
	if err != nil {
		return "", err
	}

	encDraft, err := addrKR.Encrypt(pgp.NewPlainMessageFromString(opts.Body), addrKR)
	if err != nil {
		return "", fmt.Errorf("encrypt draft: %w", err)
	}
	armoredDraft, err := encDraft.GetArmored()
	if err != nil {
		return "", err
	}
	var draft struct {
		Code    int
		Message struct{ ID string }
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/mail/v4/messages",
		Body: newDraftBody(senderAddr, opts, armoredDraft, mimeType),
	}, &draft); err != nil {
		return "", err
	}
	messageID := draft.Message.ID
	cleanup := func() {
		// The delete-messages endpoint is a PUT (see Delete); using DELETE here
		// silently failed, leaking the draft whenever a send aborted.
		_, _ = s.C.Do(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/delete", Body: map[string]any{"IDs": []string{messageID}}})
	}

	// Read attachment bytes once: they are uploaded (for internal/EO/clear
	// packages) and also embedded verbatim in the PGP/MIME body.
	prepared, err := prepareAttachments(opts.Attachments, opts.InlineAttachments)
	if err != nil {
		cleanup()
		return "", err
	}
	prepared = append(prepared, inlinePrepared...)
	var atts []*uploadedAttachment
	for _, p := range prepared {
		a, err := s.uploadAttachmentData(ctx, addrKR, messageID, p.Filename, p.MIMEType, p.ContentID, p.Data)
		if err != nil {
			cleanup()
			return "", err
		}
		atts = append(atts, a)
	}

	plans := make([]plannedRecipient, 0, len(opts.To)+len(opts.CC)+len(opts.BCC))
	needBody, hasEO := false, false
	for _, email := range dedupeRecipients(opts.To, opts.CC, opts.BCC) {
		p, err := s.planRecipient(ctx, email, opts.EOPassword, opts.PinnedKeys[email])
		if err != nil {
			cleanup()
			return "", err
		}
		plans = append(plans, p)
		// PGP/MIME and PGP-Inline recipients each get their own body package;
		// everything else shares the single internal/EO/clear body.
		if p.scheme != schemeExternalPGP && p.scheme != schemeExternalInline {
			needBody = true
		}
		if p.scheme == schemeEO {
			hasEO = true
		}
	}

	var eoModulus, eoModulusID string
	if hasEO {
		if eoModulus, eoModulusID, err = s.fetchModulus(ctx); err != nil {
			cleanup()
			return "", err
		}
	}

	var packages []map[string]any

	if needBody {
		bodyPkgs, err := s.buildBodyPackages(mimeType, opts, atts, plans, addrKR, eoModulus, eoModulusID)
		if err != nil {
			cleanup()
			return "", err
		}
		packages = append(packages, bodyPkgs...)
	}

	if pkg, ok, err := s.buildPGPMIMEPackage(opts, prepared, plans, addrKR); err != nil {
		cleanup()
		return "", err
	} else if ok {
		packages = append(packages, pkg)
	}

	if pkg, ok, err := s.buildInlinePackage(opts, atts, plans, addrKR); err != nil {
		cleanup()
		return "", err
	} else if ok {
		packages = append(packages, pkg)
	}

	sendReq := map[string]any{"ExpirationTime": nil, "AutoSaveContacts": 0, "Packages": packages}
	if opts.DeliveryTime > 0 {
		sendReq["DeliveryTime"] = opts.DeliveryTime
	}
	expiresIn := opts.ExpiresInSeconds
	if expiresIn == 0 && hasEO {
		expiresIn = defaultEOExpirationSeconds
	}
	if expiresIn > 0 {
		sendReq["ExpiresIn"] = expiresIn
	}
	resp, err := s.C.Do(ctx, proton.Request{Method: "POST", Path: "/mail/v4/messages/" + messageID, Body: sendReq})
	if err != nil {
		cleanup()
		return "", err
	}
	if resp.Status >= 400 {
		cleanup()
		return "", fmt.Errorf("send failed: %s", string(resp.Body))
	}
	return messageID, nil
}

func recipientList(emails []string) []map[string]string {
	out := make([]map[string]string, 0, len(emails))
	for _, e := range emails {
		out = append(out, map[string]string{"Address": e, "Name": e})
	}
	return out
}

// dedupeRecipients flattens To/CC/BCC into one list, dropping case-insensitive
// duplicates while preserving first-seen order.
func dedupeRecipients(lists ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, list := range lists {
		for _, e := range list {
			key := strings.ToLower(e)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}
