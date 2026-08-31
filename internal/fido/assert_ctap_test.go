//go:build !windows

package fido

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"strings"
	"sync"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/telesma-app/ctap/backend"
	"github.com/telesma-app/ctap/transport"
	"github.com/telesma-app/ctap/transport/ctaphid"
)

// The ceremony, run end to end against a key that exists only in this test
// binary: the real CTAP stack, real CTAPHID framing over an in-memory device,
// and an answer checked the way Proton checks it - the signature verified over
// the authenticator data and the hash of the clientDataJSON this package wrote.
//
// It is the closest thing to hardware that runs in CI, and it covers everything
// between the challenge arriving and the answer leaving except the USB bus.
func TestAssertAnswersTheChallenge(t *testing.T) {
	key := newSoftKey(t)
	challenge := []byte("a challenge from Proton, 32 bytes")
	o := mustParse(t, protonChallenge(challenge, key.credID, ""))

	assertion, err := o.ceremony(context.Background(), key.sign, Prompts{})
	if err != nil {
		t.Fatalf("ceremony: %v", err)
	}

	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(assertion.ClientData, &clientData); err != nil {
		t.Fatalf("client data is not JSON: %v", err)
	}
	if clientData.Type != "webauthn.get" {
		t.Errorf("type = %q, want webauthn.get", clientData.Type)
	}
	if clientData.Origin != "https://account.proton.me" {
		t.Errorf("origin = %q, want https://account.proton.me", clientData.Origin)
	}
	if want := base64.RawURLEncoding.EncodeToString(challenge); clientData.Challenge != want {
		t.Errorf("challenge = %q, want %q", clientData.Challenge, want)
	}
	if !key.verifies(assertion.AuthenticatorData, assertion.ClientData, assertion.Signature) {
		t.Error("the signature does not verify over the authenticator data and client data")
	}
	if string(assertion.CredentialID) != string(key.credID) {
		t.Errorf("credential = %x, want %x", assertion.CredentialID, key.credID)
	}
}

// A key given one credential to choose from may answer without naming it, which
// the specification allows and Proton's API does not: it wants to be told which
// credential answered.
func TestAssertNamesTheCredentialAKeyLeftOut(t *testing.T) {
	key := newSoftKey(t)
	key.omitCredential = true
	o := mustParse(t, protonChallenge([]byte("challenge"), key.credID, ""))

	assertion, err := o.ceremony(context.Background(), key.sign, Prompts{})
	if err != nil {
		t.Fatalf("ceremony: %v", err)
	}
	if string(assertion.CredentialID) != string(key.credID) {
		t.Errorf("credential = %x, want %x", assertion.CredentialID, key.credID)
	}
	if !key.verifies(assertion.AuthenticatorData, assertion.ClientData, assertion.Signature) {
		t.Error("the signature does not verify")
	}
}

// Assert is what production calls, so the test that a challenge for somebody
// else never reaches a key goes through it rather than through the seam below.
func TestAssertRefusesAChallengeForSomebodyElse(t *testing.T) {
	key := newSoftKey(t)

	_, err := Assert(context.Background(), Request{
		Options: protonChallenge([]byte("challenge"), key.credID, "evil.example"),
		Host:    "account.proton.me",
	}, Prompts{})
	if err == nil {
		t.Fatal("a challenge naming another relying party was answered")
	}
	if !strings.Contains(err.Error(), "evil.example") {
		t.Errorf("error = %v, want it to name the relying party it refused", err)
	}
	if key.asked {
		t.Error("the key was asked about a relying party it should never have heard")
	}
}

func TestAssertReportsWhatWentWrong(t *testing.T) {
	for _, tc := range []struct {
		name      string
		enumerate backend.Enumerator
		status    transport.StatusCode
		want      error
	}{
		{name: "nothing plugged in", enumerate: noKeys, want: ErrNoDevice},
		{name: "a key this user may not open", enumerate: unopenableKey, want: ErrPermission},
		{name: "a key that holds nothing for the account", status: transport.CTAP2_ERR_NO_CREDENTIALS, want: ErrNoCredential},
		{name: "a key nobody touched", status: transport.CTAP2_ERR_ACTION_TIMEOUT, want: ErrDenied},
		{name: "a ceremony called off", status: transport.CTAP2_ERR_OPERATION_DENIED, want: ErrDenied},
		{name: "a key that has locked itself", status: transport.CTAP2_ERR_PIN_BLOCKED, want: ErrPINBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := newSoftKey(t)
			key.refuseWith = tc.status
			enumerate := tc.enumerate
			if enumerate == nil {
				enumerate = key.enumerate
			}
			o := mustParse(t, protonChallenge([]byte("challenge"), key.credID, ""))
			_, err := assertVia(context.Background(), enumerate, o, []byte("{}"), Prompts{})
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// A key that insists on being verified is answered with its PIN, and a run with
// nobody to ask says so rather than hanging or failing as something else.
func TestAssertAsksForThePINOnlyWhenTheKeyInsists(t *testing.T) {
	key := newSoftKey(t)
	key.refuseWith = transport.CTAP2_ERR_PUAT_REQUIRED
	o := mustParse(t, protonChallenge([]byte("challenge"), key.credID, ""))

	asked := false
	if _, err := o.ceremony(context.Background(), key.sign, Prompts{
		PIN: func() (string, error) { asked = true; return "1234", nil },
	}); err == nil {
		t.Error("a key that never accepts its PIN ended in success")
	}
	if !asked {
		t.Error("the key asked to verify the person and nothing asked for a PIN")
	}

	_, err := o.ceremony(context.Background(), key.sign, Prompts{})
	if !errors.Is(err, ErrPINRequired) {
		t.Errorf("err = %v, want %v", err, ErrPINRequired)
	}
}

// Nothing that can block on a person does so silently.
func TestTouchIsAnnouncedBeforeAnythingBlocks(t *testing.T) {
	key := newSoftKey(t)
	o := mustParse(t, protonChallenge([]byte("challenge"), key.credID, ""))

	var said []string
	if _, err := o.ceremony(context.Background(), key.sign, Prompts{
		Touch: func(instruction string) { said = append(said, instruction) },
	}); err != nil {
		t.Fatalf("ceremony: %v", err)
	}
	if len(said) != 1 || said[0] == "" {
		t.Fatalf("touch prompts = %q, want exactly one instruction", said)
	}
}

func mustParse(t *testing.T, raw json.RawMessage) options {
	t.Helper()
	o, err := parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return o
}

// protonChallenge is an AuthenticationOptions in the shape Proton sends one:
// binary written as arrays of numbers, and the relying party named separately
// from the host that sent it.
func protonChallenge(challenge, credentialID []byte, rpID string) json.RawMessage {
	if rpID == "" {
		rpID = "account.proton.me"
	}
	numbers := func(b []byte) []int {
		out := make([]int, len(b))
		for i, v := range b {
			out[i] = int(v)
		}
		return out
	}
	raw, err := json.Marshal(map[string]any{
		"publicKey": map[string]any{
			"challenge": numbers(challenge),
			"rpId":      rpID,
			"timeout":   60000,
			"allowCredentials": []any{
				map[string]any{"id": numbers(credentialID), "type": "public-key"},
			},
			"userVerification": "discouraged",
		},
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func noKeys(context.Context) iter.Seq2[transport.Device, error] {
	return func(func(transport.Device, error) bool) {}
}

func unopenableKey(context.Context) iter.Seq2[transport.Device, error] {
	return func(yield func(transport.Device, error) bool) {
		yield(nil, fs.ErrPermission)
	}
}

// softKey is a security key made of software: it speaks CTAPHID over an
// in-memory device and signs with a key generated for the test, so the whole
// ceremony runs with nothing plugged into the machine.
type softKey struct {
	private        *ecdsa.PrivateKey
	credID         []byte
	refuseWith     transport.StatusCode
	omitCredential bool
	asked          bool
}

func newSoftKey(t *testing.T) *softKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &softKey{private: private, credID: []byte("a credential registered with Proton")}
}

// sign is the platform signer this key stands in for.
func (k *softKey) sign(ctx context.Context, o options, clientData []byte, p Prompts) (Assertion, error) {
	return assertVia(ctx, k.enumerate, o, clientData, p)
}

// enumerate plugs the key in. Each call is a fresh connection, because the
// transport that opens one closes it, and a key survives being unplugged.
func (k *softKey) enumerate(ctx context.Context) iter.Seq2[transport.Device, error] {
	return func(yield func(transport.Device, error) bool) {
		yield(ctaphid.Open(ctx, &softLink{key: k,
			outgoing: make(chan []byte, 64), closed: make(chan struct{})}))
	}
}

// verifies checks the answer the way a relying party does.
func (k *softKey) verifies(authData, clientData, signature []byte) bool {
	hash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), hash[:]...))
	return ecdsa.VerifyASN1(&k.private.PublicKey, digest[:], signature)
}

// softLink is one connection to a softKey, framing CTAPHID reports both ways.
type softLink struct {
	key *softKey

	mu       sync.Mutex
	incoming []byte
	pending  *message
	outgoing chan []byte
	closed   chan struct{}
	once     sync.Once
}

type message struct {
	cid  uint32
	cmd  byte
	want int
	data []byte
}

const (
	reportSize      = 64
	reportWithID    = reportSize + 1
	ctapHIDPing     = 0x01
	ctapHIDInit     = 0x06
	ctapHIDCBOR     = 0x10
	ctapHIDCancel   = 0x11
	cborGetAssert   = 0x02
	cborGetInfo     = 0x04
	assignedChannel = 0x01020304
)

func (l *softLink) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *softLink) Read(ctx context.Context, report []byte) (int, error) {
	select {
	case packet := <-l.outgoing:
		return copy(report, packet), nil
	case <-l.closed:
		return 0, errors.New("security key unplugged")
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (l *softLink) Write(_ context.Context, report []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.incoming = append(l.incoming, report...)
	for len(l.incoming) >= reportWithID {
		// The first byte of every write is the HID report ID, which is zero for a
		// device with a single report.
		packet := l.incoming[1:reportWithID]
		l.incoming = l.incoming[reportWithID:]
		l.packet(packet)
	}
	return len(report), nil
}

func (l *softLink) packet(p []byte) {
	cid := binary.BigEndian.Uint32(p[:4])
	if p[4]&0x80 != 0 {
		length := int(binary.BigEndian.Uint16(p[5:7]))
		data := append([]byte{}, p[7:]...)
		if len(data) > length {
			data = data[:length]
		}
		l.pending = &message{cid: cid, cmd: p[4] &^ 0x80, want: length, data: data}
	} else if l.pending != nil {
		data := p[5:]
		if room := l.pending.want - len(l.pending.data); len(data) > room {
			data = data[:room]
		}
		l.pending.data = append(l.pending.data, data...)
	}
	if l.pending != nil && len(l.pending.data) >= l.pending.want {
		done := *l.pending
		l.pending = nil
		l.answer(done)
	}
}

func (l *softLink) answer(m message) {
	switch m.cmd {
	case ctapHIDInit:
		reply := append([]byte{}, m.data...)
		reply = binary.BigEndian.AppendUint32(reply, assignedChannel)
		// Protocol version 2, device version 1.0.0, and the one capability that
		// matters: this key speaks CBOR.
		reply = append(reply, 0x02, 0x01, 0x00, 0x00, 0x04)
		l.send(m.cid, ctapHIDInit, reply)
	case ctapHIDPing:
		l.send(m.cid, ctapHIDPing, m.data)
	case ctapHIDCBOR:
		l.send(m.cid, ctapHIDCBOR, l.cbor(m.data))
	case ctapHIDCancel:
		// A cancelled command is answered by the command itself, not by this.
	}
}

func (l *softLink) cbor(payload []byte) []byte {
	if len(payload) == 0 {
		return []byte{byte(transport.CTAP1_ERR_INVALID_LENGTH)}
	}
	switch payload[0] {
	case cborGetInfo:
		info, err := cbor.Marshal(map[int]any{
			1: []string{"FIDO_2_0", "FIDO_2_1"},
			3: make([]byte, 16),
			4: map[string]bool{"up": true, "plat": false},
			6: []int{2, 1},
		})
		if err != nil {
			return []byte{byte(transport.CTAP2_ERR_INVALID_CBOR)}
		}
		return append([]byte{byte(transport.CTAP2_OK)}, info...)
	case cborGetAssert:
		return l.assertion(payload[1:])
	default:
		return []byte{byte(transport.CTAP1_ERR_INVALID_COMMAND)}
	}
}

func (l *softLink) assertion(params []byte) []byte {
	l.key.asked = true
	if l.key.refuseWith != transport.CTAP2_OK {
		return []byte{byte(l.key.refuseWith)}
	}
	var request struct {
		RPID           string `cbor:"1,keyasint"`
		ClientDataHash []byte `cbor:"2,keyasint"`
		AllowList      []struct {
			Type string `cbor:"type"`
			ID   []byte `cbor:"id"`
		} `cbor:"3,keyasint,omitempty"`
	}
	if err := cbor.Unmarshal(params, &request); err != nil {
		return []byte{byte(transport.CTAP2_ERR_INVALID_CBOR)}
	}
	held := false
	for _, allowed := range request.AllowList {
		if string(allowed.ID) == string(l.key.credID) {
			held = true
		}
	}
	if !held {
		return []byte{byte(transport.CTAP2_ERR_NO_CREDENTIALS)}
	}

	// Authenticator data: the hash of the relying party, the flag saying somebody
	// was here, and a counter. What a real key signs, in the same order.
	rpIDHash := sha256.Sum256([]byte(request.RPID))
	authData := append(rpIDHash[:], 0x01)
	authData = binary.BigEndian.AppendUint32(authData, 1)

	digest := sha256.Sum256(append(append([]byte{}, authData...), request.ClientDataHash...))
	signature, err := ecdsa.SignASN1(rand.Reader, l.key.private, digest[:])
	if err != nil {
		return []byte{byte(transport.CTAP1_ERR_OTHER)}
	}

	response := map[int]any{2: authData, 3: signature}
	if !l.key.omitCredential {
		response[1] = map[string]any{"type": "public-key", "id": l.key.credID}
	}
	encoded, err := cbor.Marshal(response)
	if err != nil {
		return []byte{byte(transport.CTAP2_ERR_INVALID_CBOR)}
	}
	return append([]byte{byte(transport.CTAP2_OK)}, encoded...)
}

// send frames a reply the way CTAPHID does: one initialisation packet carrying
// the length, then continuation packets numbered from zero.
func (l *softLink) send(cid uint32, cmd byte, payload []byte) {
	packet := make([]byte, reportSize)
	binary.BigEndian.PutUint32(packet, cid)
	packet[4] = cmd | 0x80
	binary.BigEndian.PutUint16(packet[5:], uint16(len(payload)))
	sent := copy(packet[7:], payload)
	l.outgoing <- packet

	for sequence := byte(0); sent < len(payload); sequence++ {
		packet := make([]byte, reportSize)
		binary.BigEndian.PutUint32(packet, cid)
		packet[4] = sequence
		sent += copy(packet[5:], payload[sent:])
		l.outgoing <- packet
	}
}
