package pass

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// Sharing a vault with somebody else.
//
// A vault is opened by its share key, and every item in it is sealed under that
// key. Sharing is therefore handing somebody the key itself - every rotation of
// it, since older items are sealed under older ones - encrypted to their key and
// signed with yours.
//
// Proton passes it along without being able to read it, and the signature is
// what tells the recipient the vault really came from you rather than from
// whoever happened to send the request.

// vaultInviteContext is the signature context on an invitation to somebody who
// already has a Proton account. Marking it critical means a client that does not
// understand the notation refuses the signature rather than trusting it blind.
const vaultInviteContext = "pass.invite.vault.existing-user"

// What a vault can be shared as. Proton sends these as strings.
const (
	roleManager = "1"
	roleWrite   = "2"
	roleRead    = "3"
)

// targetVault is the kind of thing an invitation points at. Items can be shared
// on their own too; this shares vaults.
const targetVault = 1

// roleWords name what somebody may do, the way --access reads.
var roleWords = map[string]string{
	roleManager: "manager", roleWrite: "editor", roleRead: "viewer",
}

// VaultRoles are the ways a vault can be shared, for --access.
func VaultRoles() []string { return []string{"viewer", "editor", "manager"} }

// roleFor turns the word somebody typed into what Proton wants.
func roleFor(access string) (string, error) {
	for id, word := range roleWords {
		if word == access {
			return id, nil
		}
	}
	return "", fmt.Errorf("unknown access %q", access)
}

// VaultInvite is somebody who has been offered a vault, or offered you one.
type VaultInvite struct {
	ID string `json:"id"`
	// ShareID is the vault the invitation is about. It is only known on your own
	// vaults: an invitation you received names a vault you cannot see yet.
	ShareID string `json:"share_id,omitempty"`
	// Vault is what the sender calls it, which is what an invitation you received
	// shows instead.
	Vault   string `json:"vault,omitempty"`
	Email   string `json:"email"`
	Inviter string `json:"inviter,omitempty"`
	Access  string `json:"access"`
	// Items is how many things are in the vault, as the sender counted them.
	Items int `json:"items,omitempty"`
}

// VaultShare offers a vault to somebody.
//
// Every rotation of the share key is sent, because an item made before the last
// rotation is still sealed under the older one - somebody given only the newest
// key would see a vault half of which will not open.
func (s *Service) VaultShare(ctx context.Context, shareID, email, access string) error {
	role, err := roleFor(access)
	if err != nil {
		return err
	}
	sk, err := s.decryptShareKeys(ctx, shareID)
	if err != nil {
		return err
	}
	u, err := s.keys(ctx)
	if err != nil {
		return err
	}
	addrKR, _, err := u.PrimaryAddr()
	if err != nil {
		return err
	}
	inviteeKR, err := s.publicKeyRing(ctx, email)
	if err != nil {
		return err
	}

	rotations := make([]int, 0, len(sk.keys))
	for r := range sk.keys {
		rotations = append(rotations, r)
	}
	sort.Ints(rotations)

	keys := make([]map[string]any, 0, len(rotations))
	for _, rotation := range rotations {
		sealed, err := inviteeKR.EncryptWithContext(
			pgp.NewPlainMessage(sk.keys[rotation]), addrKR,
			pgp.NewSigningContext(vaultInviteContext, true),
		)
		if err != nil {
			return fmt.Errorf("encrypt the vault key for %s: %w", email, err)
		}
		keys = append(keys, map[string]any{
			"Key":         base64.StdEncoding.EncodeToString(sealed.GetBinary()),
			"KeyRotation": rotation,
		})
	}

	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/pass/v1/share/" + shareID + "/invite",
		Body: map[string]any{
			"Keys": keys, "Email": email,
			"ShareRoleID": role, "TargetType": targetVault,
		},
	}, nil)
}

// publicKeyRing is the key Proton publishes for an address, refused with a
// sentence rather than an empty ring when there is none.
func (s *Service) publicKeyRing(ctx context.Context, email string) (*pgp.KeyRing, error) {
	kr, err := keys.Published(ctx, s.C, email)
	if err != nil {
		return nil, err
	}
	if kr == nil {
		return nil, errs.Problemf("%s is not a Proton address, so there is no key to share with.", email).
			Hint("a vault can only be shared with another Proton account")
	}
	return kr, nil
}

// VaultInvitesSent lists who has been offered one of your vaults and has not
// answered.
func (s *Service) VaultInvitesSent(ctx context.Context, shareID string) ([]VaultInvite, error) {
	var r struct {
		Invites []struct {
			InviteID     string
			InvitedEmail string
			InviterEmail string
			ShareRoleID  string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/pass/v1/share/" + shareID + "/invite",
	}, &r); err != nil {
		return nil, err
	}
	out := make([]VaultInvite, 0, len(r.Invites))
	for _, i := range r.Invites {
		out = append(out, VaultInvite{
			ID: i.InviteID, ShareID: shareID, Email: i.InvitedEmail,
			Inviter: i.InviterEmail, Access: roleWord(i.ShareRoleID),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

// roleWord names a role, falling back to the number for one this version has
// not been told about rather than guessing.
func roleWord(id string) string {
	if w, ok := roleWords[id]; ok {
		return w
	}
	return "role " + id
}

// VaultInviteRevoke withdraws an offer nobody has answered.
func (s *Service) VaultInviteRevoke(ctx context.Context, shareID, inviteID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "DELETE", Path: "/pass/v1/share/" + shareID + "/invite/" + inviteID,
	}, nil)
}

// VaultInvitesReceived lists the vaults other people have offered you.
func (s *Service) VaultInvitesReceived(ctx context.Context) ([]VaultInvite, error) {
	var r struct {
		Invites []rawUserInvite
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/pass/v1/invite"}, &r); err != nil {
		return nil, err
	}
	u, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]VaultInvite, 0, len(r.Invites))
	for _, i := range r.Invites {
		invite := VaultInvite{
			ID: i.InviteToken, Email: i.InvitedEmail, Inviter: i.InviterEmail,
			Access: roleWord(i.ShareRoleID), Items: i.VaultData.ItemCount,
		}
		// The vault's name is readable before the offer is taken: the invitation
		// carries the key that opens it, encrypted to the address it was sent to.
		// A name that will not come out is left empty rather than guessed at -
		// the offer is still there to answer.
		if key, err := s.openInviteKey(i, u); err == nil {
			if vault, err := decryptVault(i.VaultData.Content, key); err == nil {
				invite.Vault = vault.Name
			}
		}
		out = append(out, invite)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Inviter < out[j].Inviter })
	return out, nil
}

// rawUserInvite is an invitation as it reaches the person offered it. The vault's
// name is in encrypted content they cannot open until they accept, so Proton
// sends a preview alongside.
type rawUserInvite struct {
	InviteToken      string
	InvitedEmail     string
	InvitedAddressID string
	InviterEmail     string
	ShareRoleID      string
	TargetType       int
	Keys             []struct {
		Key         string
		KeyRotation int
	}
	VaultData struct {
		Content            string
		ContentKeyRotation int
		ItemCount          int
		MemberCount        int
	}
}

// openInviteKey unseals the vault key an invitation carries.
//
// An invitation holds every rotation of the key; the one that opens the preview
// is the rotation the preview was sealed under.
func (s *Service) openInviteKey(i rawUserInvite, u *keys.Unlocked) ([]byte, error) {
	addrKR, ok := u.AddrKR(i.InvitedAddressID)
	if !ok {
		return nil, fmt.Errorf("no key for the address this was sent to")
	}
	for _, k := range i.Keys {
		if k.KeyRotation != i.VaultData.ContentKeyRotation {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(k.Key)
		if err != nil {
			return nil, err
		}
		opened, err := addrKR.Decrypt(pgp.NewPGPMessage(raw), nil, pgp.GetUnixTime())
		if err != nil {
			return nil, err
		}
		return opened.GetBinary(), nil
	}
	return nil, fmt.Errorf("the invitation carries no key for its own preview")
}

// VaultInviteAccept takes a vault somebody offered.
//
// The keys arrive encrypted to the address the offer was sent to. They are moved
// onto the account's own user key here, which is where the CLI reads a vault's
// keys from afterwards - so accepting is what turns an offer into a vault that
// opens like any other.
func (s *Service) VaultInviteAccept(ctx context.Context, token string) error {
	invite, u, err := s.findVaultInvite(ctx, token)
	if err != nil {
		return err
	}
	addrKR, ok := u.AddrKR(invite.InvitedAddressID)
	if !ok {
		return errs.Problemf("The keys for %s will not open, so that offer cannot be taken.", invite.InvitedEmail)
	}

	keys := make([]map[string]any, 0, len(invite.Keys))
	for _, k := range invite.Keys {
		raw, err := base64.StdEncoding.DecodeString(k.Key)
		if err != nil {
			return err
		}
		opened, err := addrKR.Decrypt(pgp.NewPGPMessage(raw), nil, pgp.GetUnixTime())
		if err != nil {
			return fmt.Errorf("open the vault key sent to you: %w", err)
		}
		sealed, err := u.UserKR.Encrypt(pgp.NewPlainMessage(opened.GetBinary()), u.UserKR)
		if err != nil {
			return err
		}
		keys = append(keys, map[string]any{
			"Key":         base64.StdEncoding.EncodeToString(sealed.GetBinary()),
			"KeyRotation": k.KeyRotation,
		})
	}

	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/pass/v1/invite/" + token,
		Body: map[string]any{"Keys": keys},
	}, nil)
}

// VaultInviteReject turns an offer down. Nothing is opened: declining is saying
// no to the offer rather than reading it first.
func (s *Service) VaultInviteReject(ctx context.Context, token string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "DELETE", Path: "/pass/v1/invite/" + token,
	}, nil)
}

// findVaultInvite reads one offer whole, with the account's keys.
func (s *Service) findVaultInvite(ctx context.Context, token string) (rawUserInvite, *keys.Unlocked, error) {
	var r struct {
		Invites []rawUserInvite
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/pass/v1/invite"}, &r); err != nil {
		return rawUserInvite{}, nil, err
	}
	u, err := s.keys(ctx)
	if err != nil {
		return rawUserInvite{}, nil, err
	}
	for _, i := range r.Invites {
		if i.InviteToken == token {
			return i, u, nil
		}
	}
	return rawUserInvite{}, nil, &errs.NotFound{Kind: "invitation", Ref: token}
}
