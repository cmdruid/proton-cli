package mail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	srp "github.com/ProtonMail/go-srp"
	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/ProtonMail/gopenpgp/v2/helper"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

// Proton send-package types (PACKAGE_TYPE).
const (
	pkgInternal = 1  // SEND_PM: E2EE to a Proton user
	pkgEO       = 2  // SEND_EO: encrypted-for-outside (password link)
	pkgClear    = 4  // SEND_CLEAR: cleartext (TLS only)
	pkgPGPMIME  = 16 // SEND_PGP_MIME: encrypted to an external recipient's PGP key
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
	schemeInternal    sendScheme = iota // Proton user -> E2EE
	schemeExternalPGP                   // external user with a usable PGP key -> PGP/MIME
	schemeEO                            // external user + EO password -> password link
	schemeClear                         // external user, no key, no password -> cleartext
)

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

func (s *Service) planRecipient(ctx context.Context, email, eoPassword string) (plannedRecipient, error) {
	var resp keysAllResponse
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/keys/all",
		Query: keys.Query("Email", email, "InternalOnly", "0"),
	}, &resp); err != nil {
		return plannedRecipient{}, err
	}
	scheme, armored := classifyRecipient(resp, eoPassword)
	return plannedRecipient{email: email, scheme: scheme, armoredKey: armored}, nil
}

// Send classifies each recipient and packages the message per scheme: internal
// (E2EE), external-PGP (PGP/MIME), encrypted-for-outside (password link), or
// cleartext. Internal/EO/cleartext recipients share one symmetric body; PGP/MIME
// recipients get a separately encrypted MIME body with embedded attachments.
func (s *Service) Send(ctx context.Context, u *keys.Unlocked, opts SendOptions) error {
	addrKR, _, senderEmail, err := u.FirstAddrKR()
	if err != nil {
		return err
	}

	mimeType := "text/plain"
	if opts.HTML {
		mimeType = "text/html"
	}

	// Inline images assign a Content-ID and append a cid: reference to the body,
	// so this must happen before the draft body is encrypted below.
	inlinePrepared, err := prepareInlineImages(&opts, senderEmail)
	if err != nil {
		return err
	}

	encDraft, err := addrKR.Encrypt(pgp.NewPlainMessageFromString(opts.Body), addrKR)
	if err != nil {
		return fmt.Errorf("encrypt draft: %w", err)
	}
	armoredDraft, err := encDraft.GetArmored()
	if err != nil {
		return err
	}
	var draft struct {
		Code    int
		Message struct{ ID string }
	}
	draftBody := map[string]any{
		"Message": map[string]any{
			"ToList":   recipientList(opts.To),
			"CCList":   recipientList(opts.CC),
			"BCCList":  recipientList(opts.BCC),
			"Subject":  opts.Subject,
			"Sender":   map[string]string{"Address": senderEmail, "Name": ""},
			"Body":     armoredDraft,
			"MIMEType": mimeType,
		},
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/mail/v4/messages", Body: draftBody}, &draft); err != nil {
		return err
	}
	messageID := draft.Message.ID
	cleanup := func() {
		_, _ = s.C.Do(ctx, proton.Request{Method: "DELETE", Path: "/mail/v4/messages/delete", Body: map[string]any{"IDs": []string{messageID}}})
	}

	// Read attachment bytes once: they are uploaded (for internal/EO/clear
	// packages) and also embedded verbatim in the PGP/MIME body.
	prepared, err := prepareAttachments(opts.Attachments, opts.InlineAttachments)
	if err != nil {
		cleanup()
		return err
	}
	prepared = append(prepared, inlinePrepared...)
	var atts []*uploadedAttachment
	for _, p := range prepared {
		a, err := s.uploadAttachmentData(ctx, addrKR, messageID, p.Filename, p.MIMEType, p.ContentID, p.Data)
		if err != nil {
			cleanup()
			return err
		}
		atts = append(atts, a)
	}

	plans := make([]plannedRecipient, 0, len(opts.To)+len(opts.CC)+len(opts.BCC))
	needBody, hasEO := false, false
	for _, email := range dedupeRecipients(opts.To, opts.CC, opts.BCC) {
		p, err := s.planRecipient(ctx, email, opts.EOPassword)
		if err != nil {
			cleanup()
			return err
		}
		plans = append(plans, p)
		if p.scheme != schemeExternalPGP {
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
			return err
		}
	}

	var packages []map[string]any

	if needBody {
		bodyPkgs, err := s.buildBodyPackages(mimeType, opts, atts, plans, addrKR, eoModulus, eoModulusID)
		if err != nil {
			cleanup()
			return err
		}
		packages = append(packages, bodyPkgs...)
	}

	if pkg, ok, err := s.buildPGPMIMEPackage(opts, prepared, plans, addrKR); err != nil {
		cleanup()
		return err
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
		return err
	}
	if resp.Status >= 400 {
		cleanup()
		return fmt.Errorf("send failed: %s", string(resp.Body))
	}
	return nil
}

// buildBodyPackages encrypts the plaintext/HTML body once under a shared session
// key and returns up to three packages (internal, EO, cleartext) that reference
// it, keyed per recipient scheme.
func (s *Service) buildBodyPackages(mimeType string, opts SendOptions, atts []*uploadedAttachment, plans []plannedRecipient, addrKR *pgp.KeyRing, eoModulus, eoModulusID string) ([]map[string]any, error) {
	sessionKey, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	encBody, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(opts.Body), addrKR)
	if err != nil {
		return nil, err
	}
	bodyB64 := base64.StdEncoding.EncodeToString(encBody)

	internalAddrs := map[string]any{}
	eoAddrs := map[string]any{}
	clearAddrs := map[string]any{}

	for _, p := range plans {
		switch p.scheme {
		case schemeInternal:
			recKR, err := keyRingFromArmored(p.armoredKey)
			if err != nil {
				return nil, fmt.Errorf("parse recipient key for %s: %w", p.email, err)
			}
			recKP, err := recKR.EncryptSessionKey(sessionKey)
			if err != nil {
				return nil, err
			}
			addr := map[string]any{
				"Type":          pkgInternal,
				"BodyKeyPacket": base64.StdEncoding.EncodeToString(recKP),
				"Signature":     0,
			}
			akp, err := attachmentKeyPackets(recKR, atts)
			if err != nil {
				return nil, err
			}
			if akp != nil {
				addr["AttachmentKeyPackets"] = akp
			}
			internalAddrs[p.email] = addr
		case schemeEO:
			addr, err := eoAddress(sessionKey, opts.EOPassword, opts.EOPasswordHint, atts, eoModulus, eoModulusID)
			if err != nil {
				return nil, err
			}
			eoAddrs[p.email] = addr
		case schemeClear:
			clearAddrs[p.email] = map[string]any{"Type": pkgClear, "Signature": 0}
		}
	}

	var packages []map[string]any
	if len(internalAddrs) > 0 {
		bodyKP, err := addrKR.EncryptSessionKey(sessionKey)
		if err != nil {
			return nil, err
		}
		packages = append(packages, map[string]any{
			"Addresses":     internalAddrs,
			"MIMEType":      mimeType,
			"Type":          pkgInternal,
			"Body":          bodyB64,
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		})
	}
	if len(eoAddrs) > 0 {
		packages = append(packages, map[string]any{
			"Addresses": eoAddrs,
			"MIMEType":  mimeType,
			"Type":      pkgEO,
			"Body":      bodyB64,
		})
	}
	if len(clearAddrs) > 0 {
		clearPkg := map[string]any{
			"Addresses": clearAddrs,
			"MIMEType":  mimeType,
			"Type":      pkgClear,
			"Body":      bodyB64,
			"BodyKey":   map[string]any{"Key": base64.StdEncoding.EncodeToString(sessionKey.Key), "Algorithm": sessionKey.Algo},
		}
		if ak := attachmentCleartextKeys(atts); ak != nil {
			clearPkg["AttachmentKeys"] = ak
		}
		packages = append(packages, clearPkg)
	}
	return packages, nil
}

// buildPGPMIMEPackage builds the multipart/mixed MIME body (with embedded
// attachments), encrypts it under a fresh session key, and wraps that key to
// each external-PGP recipient's key. Returns ok=false when no recipient uses
// PGP/MIME.
func (s *Service) buildPGPMIMEPackage(opts SendOptions, prepared []preparedAttachment, plans []plannedRecipient, addrKR *pgp.KeyRing) (map[string]any, bool, error) {
	mimeType := "text/plain"
	if opts.HTML {
		mimeType = "text/html"
	}
	addrs := map[string]any{}
	var sessionKey *pgp.SessionKey
	var bodyB64 string

	for _, p := range plans {
		if p.scheme != schemeExternalPGP {
			continue
		}
		if sessionKey == nil {
			mimeStr, err := buildMIMEMessage(opts.Body, mimeType, prepared)
			if err != nil {
				return nil, false, err
			}
			sessionKey, err = pgp.GenerateSessionKey()
			if err != nil {
				return nil, false, err
			}
			enc, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(mimeStr), addrKR)
			if err != nil {
				return nil, false, err
			}
			bodyB64 = base64.StdEncoding.EncodeToString(enc)
		}
		recKR, err := keyRingFromArmored(p.armoredKey)
		if err != nil {
			return nil, false, fmt.Errorf("parse recipient key for %s: %w", p.email, err)
		}
		kp, err := recKR.EncryptSessionKey(sessionKey)
		if err != nil {
			return nil, false, err
		}
		addrs[p.email] = map[string]any{
			"Type":          pkgPGPMIME,
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(kp),
		}
	}
	if len(addrs) == 0 {
		return nil, false, nil
	}
	return map[string]any{
		"Addresses": addrs,
		"MIMEType":  "multipart/mixed",
		"Type":      pkgPGPMIME,
		"Body":      bodyB64,
	}, true, nil
}

// eoAddress builds an encrypted-for-outside sub-package: the body session key
// and attachment keys are wrapped with the password, and a random token plus an
// SRP verifier let the recipient authenticate to Proton's EO viewer.
func eoAddress(sessionKey *pgp.SessionKey, password, hint string, atts []*uploadedAttachment, modulus, modulusID string) (map[string]any, error) {
	bodyKP, err := pgp.EncryptSessionKeyWithPassword(sessionKey, []byte(password))
	if err != nil {
		return nil, err
	}
	tokenSK, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	token := base64.StdEncoding.EncodeToString(tokenSK.Key)
	encToken, err := helper.EncryptMessageWithPassword([]byte(password), token)
	if err != nil {
		return nil, err
	}
	auth, err := eoAuth(password, modulus, modulusID)
	if err != nil {
		return nil, err
	}
	addr := map[string]any{
		"Type":          pkgEO,
		"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		"Token":         token,
		"EncToken":      encToken,
		"Auth":          auth,
		"Signature":     0,
	}
	if hint != "" {
		addr["PasswordHint"] = hint
	}
	akp, err := attachmentPasswordKeyPackets(atts, password)
	if err != nil {
		return nil, err
	}
	if akp != nil {
		addr["AttachmentKeyPackets"] = akp
	}
	return addr, nil
}

// eoAuth builds the SRP verifier the EO recipient uses to prove knowledge of the
// password, reusing the shared modulus with a fresh per-recipient salt.
func eoAuth(password, modulus, modulusID string) (map[string]any, error) {
	salt := make([]byte, 10)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	auth, err := srp.NewAuthForVerifier([]byte(password), modulus, salt)
	if err != nil {
		return nil, err
	}
	verifier, err := auth.GenerateVerifier(2048)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Version":   srpAuthVersion,
		"ModulusID": modulusID,
		"Salt":      base64.StdEncoding.EncodeToString(salt),
		"Verifier":  base64.StdEncoding.EncodeToString(verifier),
	}, nil
}

func (s *Service) fetchModulus(ctx context.Context) (modulus, modulusID string, err error) {
	var m struct{ Modulus, ModulusID string }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/auth/modulus"}, &m); err != nil {
		return "", "", err
	}
	return m.Modulus, m.ModulusID, nil
}

func keyRingFromArmored(armored string) (*pgp.KeyRing, error) {
	key, err := pgp.NewKeyFromArmored(armored)
	if err != nil {
		return nil, err
	}
	return pgp.NewKeyRing(key)
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
