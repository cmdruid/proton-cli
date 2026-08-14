package drive

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// OnConflict is what to do when the folder already holds the name being written.
//
// The three answers are the ones Proton's own client offers when it finds a
// duplicate: replace what is there, keep both, or leave it alone
// (TransferConflictStrategy, applications/drive UploadConflictModal).
type OnConflict string

const (
	// ConflictRefuse is the absence of an answer. Nothing is checked and nothing
	// is guessed: Proton refuses the write and the caller says which name was
	// taken, which is the CLI's version of the question the web asks.
	ConflictRefuse OnConflict = ""
	// ConflictRename keeps both by adding a number to the name being written,
	// leaving what is already there untouched.
	ConflictRename OnConflict = "rename"
	// ConflictReplace writes the bytes as a new revision of the file already
	// there, so the file keeps its identity and its history.
	ConflictReplace OnConflict = "replace"
	// ConflictSkip leaves what is there alone and writes nothing.
	ConflictSkip OnConflict = "skip"
)

// UploadPlan is what an upload will do, worked out before it does it: which
// folder receives the bytes, which name they land under, and whether they become
// a new file, a new revision of the one already there, or nothing at all.
//
// Planning is a step of its own because a command has to report what it did and a
// dry run has to promise what it would do, and both are sentences about the
// answer to the duplicate question rather than about the request.
type UploadPlan struct {
	// Name is what the file will be called, which is not what was asked for when
	// both are being kept.
	Name string
	// Dest is the folder that receives it, as the caller named it.
	Dest string
	// Revision reports that the bytes become a new revision of the file already
	// there rather than a new file.
	Revision bool
	// Nothing reports that the file is already there and is being left alone.
	Nothing bool

	parent *Resolved
	hash   string
	// target is the file a new revision is being written to, with the revision
	// that is current now: Proton wants to be told which one is being succeeded.
	target *Resolved
	from   string
}

// PlanUpload decides what uploading name into destPath will do.
//
// Without an answer to the duplicate question there is nothing to look up: the
// upload goes ahead and Proton's own refusal is the answer, so the common case
// costs no extra request. With one, the names in the way are checked first,
// which is the same order the web client works in - it asks what to do only
// once it knows there is a duplicate.
func (s *Service) PlanUpload(ctx context.Context, dc *Context, destPath, name string, on OnConflict) (*UploadPlan, error) {
	parent, err := s.ResolvePath(ctx, dc, destPath)
	if err != nil {
		return nil, fmt.Errorf("target folder not found: %w", err)
	}
	if !parent.IsFolder {
		return nil, fmt.Errorf("%s is not a folder", destPath)
	}
	hashKey, err := hashKeyOf(parent.Link, parent.NodeKR)
	if err != nil {
		return nil, err
	}
	hash, err := lookupHash(strings.ToLower(name), hashKey)
	if err != nil {
		return nil, err
	}
	plan := &UploadPlan{Name: name, Dest: destPath, parent: parent, hash: hash}
	if on == ConflictRefuse {
		return plan, nil
	}

	free, err := s.freeNames(ctx, parent, hashKey, name)
	if err != nil {
		return nil, err
	}
	if free.taken == "" {
		return plan, nil
	}
	switch on {
	case ConflictSkip:
		plan.Nothing = true
	case ConflictRename:
		plan.Name, plan.hash = free.name, free.hash
	case ConflictReplace:
		target, err := s.ResolvePath(ctx, dc, strings.TrimSuffix(destPath, "/")+"/"+name)
		if err != nil {
			return nil, err
		}
		// A folder in the way is not something a new revision can be written to,
		// and trashing it would be a deletion this command never announced.
		if target.IsFolder {
			return nil, &errs.Exists{Kind: "folder", Name: name, Where: destPath}
		}
		if target.Link.FileProperties == nil {
			return nil, fmt.Errorf("%s: no file properties", name)
		}
		plan.Revision, plan.target, plan.from = true, target, target.Link.FileProperties.ActiveRevision.ID
	}
	return plan, nil
}

// available is the answer to "is this name free, and if not, what is": the name
// asked for when it is free, and otherwise the first numbered variant that is.
type available struct {
	// taken is the name that is in the way, empty when nothing is.
	taken string
	name  string
	hash  string
}

// freeNames asks Proton which of a batch of candidate names are free.
//
// The candidates are the name itself and then the numbered variants of it, ten
// at a time, exactly as the web client does: it is one small request for the
// question "is this taken" and the question "what should it be called instead",
// which are the same question asked of different names.
//
// Only names Proton reports as available are treated as free. A name held by
// somebody's unfinished upload is left alone, because this client has no way to
// tell its own abandoned draft from another's.
func (s *Service) freeNames(ctx context.Context, parent *Resolved, hashKey []byte, name string) (available, error) {
	base, ext := splitName(name)
	for start := 0; start < 100; start += hashBatch {
		candidates := make([]available, 0, hashBatch)
		hashes := make([]string, 0, hashBatch)
		for i := start; i < start+hashBatch; i++ {
			candidate := numberedName(i, base, ext)
			hash, err := lookupHash(strings.ToLower(candidate), hashKey)
			if err != nil {
				return available{}, err
			}
			candidates = append(candidates, available{name: candidate, hash: hash})
			hashes = append(hashes, hash)
		}
		var r struct{ AvailableHashes []string }
		if err := s.C.Decode(ctx, proton.Request{
			Method: "POST",
			Path:   fmt.Sprintf("/drive/shares/%s/links/%s/checkAvailableHashes", parent.ShareID, parent.LinkID),
			Body:   map[string]any{"Hashes": hashes},
		}, &r); err != nil {
			return available{}, err
		}
		isFree := map[string]bool{}
		for _, h := range r.AvailableHashes {
			isFree[h] = true
		}
		for i, c := range candidates {
			if !isFree[c.hash] {
				continue
			}
			if start == 0 && i == 0 {
				return available{}, nil
			}
			return available{taken: name, name: c.name, hash: c.hash}, nil
		}
	}
	return available{}, fmt.Errorf("no free name for %q in %s", name, parent.Name)
}

// hashBatch is how many candidate names are checked per request, matching the
// web client's HASH_CHECK_AMOUNT.
const hashBatch = 10

// numberedName is the name a numbered copy takes, the same string the web client
// builds (adjustName, packages/drive-store/store/_links/link.ts): the number goes
// before the extension, in brackets, after a space.
func numberedName(i int, base, ext string) string {
	if i == 0 {
		if ext == "" {
			return base
		}
		return base + "." + ext
	}
	if base == "" {
		return fmt.Sprintf(".%s (%d)", ext, i)
	}
	if ext == "" {
		return fmt.Sprintf("%s (%d)", base, i)
	}
	return fmt.Sprintf("%s (%d).%s", base, i, ext)
}

// splitName splits a name into the part a number is added to and the extension,
// which a trailing dot and a leading dot both belong to the name itself
// (splitLinkName, the same file).
func splitName(name string) (base, ext string) {
	if strings.HasSuffix(name, ".") {
		return name, ""
	}
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// startRevision opens the revision the bytes will be written to and hands back
// the keys they are encrypted with.
//
// A new file brings its own keys. A new revision of a file that already exists
// uses that file's keys, because a revision is another version of the same
// encrypted thing rather than a new one.
func (s *Service) startRevision(ctx context.Context, dc *Context, plan *UploadPlan, mimeType string) (
	linkID, revisionID string, sessionKey *pgp.SessionKey, nodeKR *pgp.KeyRing, err error,
) {
	if plan.Revision {
		link := plan.target
		kp, err := base64.StdEncoding.DecodeString(link.Link.FileProperties.ContentKeyPacket)
		if err != nil {
			return "", "", nil, nil, err
		}
		sessionKey, err := link.NodeKR.DecryptSessionKey(kp)
		if err != nil {
			return "", "", nil, nil, fmt.Errorf("get file session key: %w", err)
		}
		var r struct{ Revision struct{ ID string } }
		if err := s.C.Decode(ctx, proton.Request{
			Method: "POST",
			Path:   fmt.Sprintf("/drive/shares/%s/files/%s/revisions", link.ShareID, link.LinkID),
			Body:   map[string]any{"CurrentRevisionID": plan.from},
		}, &r); err != nil {
			return "", "", nil, nil, err
		}
		return link.LinkID, r.Revision.ID, sessionKey, link.NodeKR, nil
	}

	parent := plan.parent
	encName, err := encryptName(plan.Name, parent.NodeKR, dc.AddrKR)
	if err != nil {
		return "", "", nil, nil, err
	}
	nodeKey, nodePass, nodePassSig, nodePriv, err := genNodeKeys(parent.NodeKR, dc.AddrKR)
	if err != nil {
		return "", "", nil, nil, err
	}
	nodeKR, err = pgp.NewKeyRing(nodePriv)
	if err != nil {
		return "", "", nil, nil, err
	}
	sessionKey, contentKP, contentKPSig, err := genFileKeys(nodeKR, dc.AddrKR)
	if err != nil {
		return "", "", nil, nil, err
	}
	var created struct {
		File struct{ ID, RevisionID string }
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/drive/shares/" + parent.ShareID + "/files",
		Body: map[string]any{
			"Name": encName, "Hash": plan.hash,
			"ParentLinkID":   parent.LinkID,
			"NodePassphrase": nodePass, "NodePassphraseSignature": nodePassSig,
			"SignatureAddress":          dc.AddrEmail,
			"NodeKey":                   nodeKey,
			"MIMEType":                  mimeType,
			"ContentKeyPacket":          contentKP,
			"ContentKeyPacketSignature": contentKPSig,
		},
	}, &created); err != nil {
		// Proton is the one that knows the name is taken when nobody asked it to
		// look, so this is where that answer gets its words.
		if proton.AlreadyExists(err) {
			return "", "", nil, nil, &errs.Exists{Kind: "file", Name: plan.Name, Where: plan.Dest}
		}
		return "", "", nil, nil, err
	}
	return created.File.ID, created.File.RevisionID, sessionKey, nodeKR, nil
}
