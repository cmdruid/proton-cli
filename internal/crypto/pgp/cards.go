// Package pgp wraps gopenpgp primitives Proton uses for Calendar events,
// Contacts, Drive links and Pass key blobs. Nothing here talks to the API.
package pgp

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
)

// CalendarKeyPayload is the calendar key-setup data for
// POST /calendar/v1/{id}/keys.
type CalendarKeyPayload struct {
	PrivateKey string // armored calendar private key, locked with a generated passphrase
	DataPacket string // base64 symmetric ciphertext of the passphrase
	KeyPacket  string // base64 session key encrypted to the address public key
	Signature  string // armored detached signature over the passphrase
}

// GenerateCalendarKey creates a fresh calendar key and the split
// (encrypted + signed) passphrase bound to the given address key ring, matching
// Proton's calendar key-setup flow.
func GenerateCalendarKey(addrKR *pgp.KeyRing) (*CalendarKeyPayload, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	passphrase := base64.StdEncoding.EncodeToString(raw)

	key, err := pgp.GenerateKey("Calendar key", "", "x25519", 0)
	if err != nil {
		return nil, err
	}
	locked, err := key.Lock([]byte(passphrase))
	if err != nil {
		return nil, err
	}
	privArmored, err := locked.Armor()
	if err != nil {
		return nil, err
	}

	msg := pgp.NewPlainMessageFromString(passphrase)
	sk, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	dataPacket, err := sk.Encrypt(msg)
	if err != nil {
		return nil, err
	}
	keyPacket, err := addrKR.EncryptSessionKey(sk)
	if err != nil {
		return nil, err
	}
	sig, err := addrKR.SignDetached(msg)
	if err != nil {
		return nil, err
	}
	sigArmored, err := sig.GetArmored()
	if err != nil {
		return nil, err
	}
	return &CalendarKeyPayload{
		PrivateKey: privArmored,
		DataPacket: base64.StdEncoding.EncodeToString(dataPacket),
		KeyPacket:  base64.StdEncoding.EncodeToString(keyPacket),
		Signature:  sigArmored,
	}, nil
}

// Card types used by Proton for its VEVENT / VCard blobs.
const (
	CardClear           = 0
	CardEncrypted       = 1
	CardSigned          = 2
	CardEncryptedSigned = 3
)

// Card is a Proton-style signed/encrypted blob as returned by the API.
type Card struct {
	Type      int    `json:"Type"`
	Data      string `json:"Data"`
	Signature string `json:"Signature,omitempty"`
}

// DecryptCards decrypts a list of mixed-type cards. decryptionKR decrypts
// types 1/3, verificationKR is used to (best-effort) verify type 2 signatures.
// If keyPacket is set, it is used as a prefix for types 1/3 whose Data field
// contains only the data packet (Proton calendar shared events).
//
// Alongside the decrypted strings it returns a per-card verdict: type-2/3 cards
// are signature-checked against verificationKR; clear/encrypted-only cards are
// reported Unsigned. Callers typically pgp.Aggregate the verdicts.
func DecryptCards(cards []Card, decryptionKR, verificationKR *pgp.KeyRing, keyPacket []byte) ([]string, []VerifyResult, error) {
	out := make([]string, 0, len(cards))
	verdicts := make([]VerifyResult, 0, len(cards))
	for _, c := range cards {
		switch c.Type {
		case CardClear:
			out = append(out, c.Data)
			verdicts = append(verdicts, Unsigned)
		case CardSigned:
			out = append(out, c.Data)
			verdicts = append(verdicts, VerifyDetachedStatus(verificationKR, pgp.NewPlainMessageFromString(c.Data), c.Signature))
		case CardEncrypted:
			plain, err := decryptCardData(c.Data, keyPacket, decryptionKR)
			if err != nil {
				return nil, nil, fmt.Errorf("decrypt card (type %d): %w", c.Type, err)
			}
			out = append(out, plain)
			verdicts = append(verdicts, Unsigned)
		case CardEncryptedSigned:
			plain, err := decryptCardData(c.Data, keyPacket, decryptionKR)
			if err != nil {
				return nil, nil, fmt.Errorf("decrypt card (type %d): %w", c.Type, err)
			}
			out = append(out, plain)
			verdicts = append(verdicts, VerifyDetachedStatus(verificationKR, pgp.NewPlainMessageFromString(plain), c.Signature))
		default:
			out = append(out, c.Data)
			verdicts = append(verdicts, Unsigned)
		}
	}
	return out, verdicts, nil
}

// DecryptCardsRaw accepts the map form returned from json.Unmarshal.
func DecryptCardsRaw(cards []map[string]any, decryptionKR, verificationKR *pgp.KeyRing, keyPacket []byte) ([]string, []VerifyResult, error) {
	typed := make([]Card, 0, len(cards))
	for _, m := range cards {
		c := Card{}
		if v, ok := m["Type"].(float64); ok {
			c.Type = int(v)
		}
		if v, ok := m["Data"].(string); ok {
			c.Data = v
		}
		if v, ok := m["Signature"].(string); ok {
			c.Signature = v
		}
		typed = append(typed, c)
	}
	return DecryptCards(typed, decryptionKR, verificationKR, keyPacket)
}

func decryptCardData(data string, keyPacket []byte, kr *pgp.KeyRing) (string, error) {
	if keyPacket != nil {
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			msg, err := pgp.NewPGPMessageFromArmored(data)
			if err != nil {
				return "", err
			}
			dec, err := kr.Decrypt(msg, nil, pgp.GetUnixTime())
			if err != nil {
				return "", err
			}
			return dec.GetString(), nil
		}
		split := pgp.NewPGPSplitMessage(keyPacket, raw)
		dec, err := kr.Decrypt(split.GetPGPMessage(), nil, pgp.GetUnixTime())
		if err != nil {
			return "", err
		}
		return dec.GetString(), nil
	}
	msg, err := pgp.NewPGPMessageFromArmored(data)
	if err != nil {
		return "", err
	}
	dec, err := kr.Decrypt(msg, nil, pgp.GetUnixTime())
	if err != nil {
		return "", err
	}
	return dec.GetString(), nil
}

func SignCard(data string, signingKR *pgp.KeyRing) (*Card, error) {
	sig, err := signingKR.SignDetached(pgp.NewPlainMessageFromString(data))
	if err != nil {
		return nil, fmt.Errorf("sign card: %w", err)
	}
	armored, err := sig.GetArmored()
	if err != nil {
		return nil, err
	}
	return &Card{Type: CardSigned, Data: data, Signature: armored}, nil
}

// EncryptAndSignCard produces a card of type CardEncryptedSigned where the
// encrypted payload is armored (no separate key packet). Used by Contacts.
func EncryptAndSignCard(data string, encryptionKR, signingKR *pgp.KeyRing) (*Card, error) {
	msg := pgp.NewPlainMessageFromString(data)
	enc, err := encryptionKR.Encrypt(msg, nil)
	if err != nil {
		return nil, fmt.Errorf("encrypt card: %w", err)
	}
	armored, err := enc.GetArmored()
	if err != nil {
		return nil, err
	}
	sig, err := signingKR.SignDetached(msg)
	if err != nil {
		return nil, err
	}
	sigArmored, err := sig.GetArmored()
	if err != nil {
		return nil, err
	}
	return &Card{Type: CardEncryptedSigned, Data: armored, Signature: sigArmored}, nil
}

// EncryptAndSignCardSplit produces (signedCard, encryptedCard, sharedKeyPacket,
// sessionKey) for Proton calendar events. When existingKeyPacket is non-empty,
// the existing session key is reused (update flow) and the returned keyPacket
// is empty. The returned session key lets callers encrypt sibling parts (e.g.
// the attendees part) and wrap it to additional recipients.
func EncryptAndSignCardSplit(signedData, encryptedData string, encryptionKR, signingKR *pgp.KeyRing, existingKeyPacketB64 string) (signed, encrypted *Card, keyPacketB64 string, sessionKey *pgp.SessionKey, err error) {
	signed, err = SignCard(signedData, signingKR)
	if err != nil {
		return nil, nil, "", nil, err
	}

	encMsg := pgp.NewPlainMessageFromString(encryptedData)
	var dataPacket []byte
	if existingKeyPacketB64 != "" {
		kpBytes, err := base64.StdEncoding.DecodeString(existingKeyPacketB64)
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf("decode existing key packet: %w", err)
		}
		sessionKey, err = encryptionKR.DecryptSessionKey(kpBytes)
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf("decrypt existing session key: %w", err)
		}
		dataPacket, err = sessionKey.Encrypt(encMsg)
		if err != nil {
			return nil, nil, "", nil, err
		}
	} else {
		enc, err := encryptionKR.Encrypt(encMsg, signingKR)
		if err != nil {
			return nil, nil, "", nil, err
		}
		split, err := enc.SplitMessage()
		if err != nil {
			return nil, nil, "", nil, err
		}
		keyPacketB64 = base64.StdEncoding.EncodeToString(split.GetBinaryKeyPacket())
		dataPacket = split.GetBinaryDataPacket()
		sessionKey, err = encryptionKR.DecryptSessionKey(split.GetBinaryKeyPacket())
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf("recover session key: %w", err)
		}
	}

	sig, err := signingKR.SignDetached(encMsg)
	if err != nil {
		return nil, nil, "", nil, err
	}
	sigArmored, err := sig.GetArmored()
	if err != nil {
		return nil, nil, "", nil, err
	}
	encrypted = &Card{Type: CardEncryptedSigned, Data: base64.StdEncoding.EncodeToString(dataPacket), Signature: sigArmored}
	return signed, encrypted, keyPacketB64, sessionKey, nil
}

// EncryptPartWithSessionKey encrypts data under an existing session key
// (encrypt-only data packet) and attaches a detached signature, matching
// Proton's ENCRYPTED_AND_SIGNED card layout for sibling event parts such as the
// attendees part.
func EncryptPartWithSessionKey(data string, sessionKey *pgp.SessionKey, signingKR *pgp.KeyRing) (*Card, error) {
	msg := pgp.NewPlainMessageFromString(data)
	dataPacket, err := sessionKey.Encrypt(msg)
	if err != nil {
		return nil, err
	}
	sig, err := signingKR.SignDetached(msg)
	if err != nil {
		return nil, err
	}
	sigArmored, err := sig.GetArmored()
	if err != nil {
		return nil, err
	}
	return &Card{Type: CardEncryptedSigned, Data: base64.StdEncoding.EncodeToString(dataPacket), Signature: sigArmored}, nil
}
