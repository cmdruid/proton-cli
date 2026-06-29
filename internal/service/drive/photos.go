package drive

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/proton"
)

type Photo struct {
	LinkID      string `json:"link_id"`
	CaptureTime int64  `json:"capture_time"`
	Hash        string `json:"hash,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Tags        []int  `json:"tags,omitempty"`
}

type Album struct {
	LinkID     string `json:"link_id"`
	Name       string `json:"name"`
	PhotoCount int    `json:"photo_count"`
}

const photosPageSize = 500

// photosRoot fetches the photos share root link and unwraps its node key ring,
// the parent for every photo and album.
func (s *Service) photosRoot(ctx context.Context, dc *Context) (*Link, *pgp.KeyRing, error) {
	root, err := s.getLink(ctx, dc.ShareID, dc.RootLinkID)
	if err != nil {
		return nil, nil, err
	}
	kr, err := unlockNode(root, dc.ShareKR, dc.AddrKR)
	if err != nil {
		return nil, nil, fmt.Errorf("unlock photos root: %w", err)
	}
	return root, kr, nil
}

// PhotosList returns all photos on the photos volume (paginated server-side).
func (s *Service) PhotosList(ctx context.Context, dc *Context) ([]Photo, error) {
	var out []Photo
	lastID := ""
	for {
		q := proton.Request{Method: "GET", Path: fmt.Sprintf("/drive/volumes/%s/photos", dc.VolumeID)}
		q.Query = map[string][]string{"PageSize": {fmt.Sprintf("%d", photosPageSize)}}
		if lastID != "" {
			q.Query.Set("PreviousPageLastLinkID", lastID)
		}
		var r struct {
			Photos []struct {
				LinkID      string
				CaptureTime int64
				Hash        string
				ContentHash string
				Tags        []int
			}
		}
		if err := s.C.Decode(ctx, q, &r); err != nil {
			return nil, err
		}
		for _, p := range r.Photos {
			out = append(out, Photo{LinkID: p.LinkID, CaptureTime: p.CaptureTime, Hash: p.Hash, ContentHash: p.ContentHash, Tags: p.Tags})
		}
		if len(r.Photos) < photosPageSize {
			break
		}
		lastID = r.Photos[len(r.Photos)-1].LinkID
	}
	return out, nil
}

// AlbumsList returns the photo albums. The list endpoint omits the (encrypted)
// album name, so each album's link is fetched and its name decrypted with the
// photos-root key.
func (s *Service) AlbumsList(ctx context.Context, dc *Context) ([]Album, error) {
	_, rootKR, err := s.photosRoot(ctx, dc)
	if err != nil {
		return nil, err
	}
	var r struct {
		Albums []struct {
			LinkID     string
			PhotoCount int
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums", dc.VolumeID)}, &r); err != nil {
		return nil, err
	}
	out := make([]Album, 0, len(r.Albums))
	for _, a := range r.Albums {
		name := ""
		if link, err := s.getLink(ctx, dc.ShareID, a.LinkID); err == nil {
			if n, derr := decryptName(link.Name, rootKR); derr == nil {
				name = n
			}
		}
		out = append(out, Album{LinkID: a.LinkID, Name: name, PhotoCount: a.PhotoCount})
	}
	return out, nil
}

// AlbumItems lists the photos in an album.
func (s *Service) AlbumItems(ctx context.Context, dc *Context, albumLinkID string) ([]Photo, error) {
	var out []Photo
	anchor := ""
	for {
		q := proton.Request{Method: "GET", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums/%s/children", dc.VolumeID, albumLinkID)}
		if anchor != "" {
			q.Query = map[string][]string{"AnchorID": {anchor}}
		}
		var r struct {
			Photos []struct {
				LinkID      string
				CaptureTime int64
				Hash        string
				Tags        []int
			}
			AnchorID string
			More     bool
		}
		if err := s.C.Decode(ctx, q, &r); err != nil {
			return nil, err
		}
		for _, p := range r.Photos {
			out = append(out, Photo{LinkID: p.LinkID, CaptureTime: p.CaptureTime, Hash: p.Hash, Tags: p.Tags})
		}
		if !r.More || r.AnchorID == "" {
			break
		}
		anchor = r.AnchorID
	}
	return out, nil
}

// PhotoDownload streams and decrypts a photo by its link ID.
func (s *Service) PhotoDownload(ctx context.Context, dc *Context, linkID string, w io.Writer, opts DownloadOptions) (string, error) {
	root, rootKR, err := s.photosRoot(ctx, dc)
	if err != nil {
		return "", err
	}
	link, err := s.getLink(ctx, dc.ShareID, linkID)
	if err != nil {
		return "", err
	}
	parentKR := rootKR
	if link.ParentLinkID != "" && link.ParentLinkID != root.LinkID {
		parentLink, err := s.getLink(ctx, dc.ShareID, link.ParentLinkID)
		if err != nil {
			return "", err
		}
		if parentKR, err = unlockNode(parentLink, rootKR, dc.AddrKR); err != nil {
			return "", err
		}
	}
	name, err := decryptName(link.Name, parentKR)
	if err != nil {
		name = linkID
	}
	nodeKR, err := unlockNode(link, parentKR, dc.AddrKR)
	if err != nil {
		return "", err
	}
	return name, s.downloadFile(ctx, dc.ShareID, link, nodeKR, w, opts)
}

// PhotoUpload uploads a file to the photos volume, marking the revision as a
// photo captured at captureTime (Unix seconds). The required ContentHash is the
// HMAC of the content's SHA-1 digest under the photos-root hash key (used for
// duplicate detection), matching the web client.
func (s *Service) PhotoUpload(ctx context.Context, dc *Context, name string, r io.Reader, captureTime int64, opts UploadOptions) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	root, rootKR, err := s.photosRoot(ctx, dc)
	if err != nil {
		return err
	}
	hashKey, err := hashKeyOf(root, rootKR)
	if err != nil {
		return err
	}
	sum := sha1.Sum(data) //nolint:gosec // Proton uses SHA-1 for the content digest
	contentHash, err := lookupHash(hex.EncodeToString(sum[:]), hashKey)
	if err != nil {
		return err
	}
	opts.Photo = map[string]any{
		"MainPhotoLinkID": nil,
		"CaptureTime":     captureTime,
		"ContentHash":     contentHash,
	}
	return s.Upload(ctx, dc, "/", name, bytes.NewReader(data), opts)
}

// AlbumCreate creates a new (unlocked) photo album.
func (s *Service) AlbumCreate(ctx context.Context, dc *Context, name string) error {
	root, rootKR, err := s.photosRoot(ctx, dc)
	if err != nil {
		return err
	}
	hashKey, err := hashKeyOf(root, rootKR)
	if err != nil {
		return err
	}
	hash, err := lookupHash(strings.ToLower(name), hashKey)
	if err != nil {
		return err
	}
	encName, err := encryptName(name, rootKR, dc.AddrKR)
	if err != nil {
		return err
	}
	nodeKey, nodePass, nodePassSig, nodePriv, err := genNodeKeys(rootKR, dc.AddrKR)
	if err != nil {
		return err
	}
	nodeKR, err := pgp.NewKeyRing(nodePriv)
	if err != nil {
		return err
	}
	nodeHashKey, err := genNodeHashKey(nodeKR, nodeKR)
	if err != nil {
		return err
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums", dc.VolumeID),
		Body: map[string]any{
			"Locked": false,
			"Link": map[string]any{
				"Name": encName, "Hash": hash,
				"NodePassphrase": nodePass, "NodePassphraseSignature": nodePassSig,
				"SignatureEmail": dc.AddrEmail,
				"NodeKey":        nodeKey, "NodeHashKey": nodeHashKey,
			},
		},
	}, nil)
}

// AlbumAddPhotos adds existing timeline photos to an album. Each photo's node
// passphrase and name are re-encrypted to the album's node key (the same
// re-wrap used by Copy), and a fresh name hash is computed against the album's
// hash key.
func (s *Service) AlbumAddPhotos(ctx context.Context, dc *Context, albumLinkID string, photoLinkIDs []string) error {
	_, rootKR, err := s.photosRoot(ctx, dc)
	if err != nil {
		return err
	}
	albumLink, err := s.getLink(ctx, dc.ShareID, albumLinkID)
	if err != nil {
		return err
	}
	albumKR, err := unlockNode(albumLink, rootKR, dc.AddrKR)
	if err != nil {
		return fmt.Errorf("unlock album: %w", err)
	}
	albumHashKey, err := hashKeyOf(albumLink, albumKR)
	if err != nil {
		return err
	}
	// The album-add payload requires each photo's ContentHash; reuse the photo's
	// existing one (the web client's documented fallback).
	contentHashes := map[string]string{}
	photos, err := s.PhotosList(ctx, dc)
	if err != nil {
		return err
	}
	for _, p := range photos {
		contentHashes[p.LinkID] = p.ContentHash
	}
	var data []map[string]any
	for _, pid := range photoLinkIDs {
		photoLink, err := s.getLink(ctx, dc.ShareID, pid)
		if err != nil {
			return err
		}
		name, err := decryptName(photoLink.Name, rootKR)
		if err != nil {
			return fmt.Errorf("decrypt photo name %s: %w", pid, err)
		}
		encName, err := reEncryptName(photoLink.Name, name, rootKR, albumKR, dc.AddrKR)
		if err != nil {
			return err
		}
		hash, err := lookupHash(strings.ToLower(name), albumHashKey)
		if err != nil {
			return err
		}
		newPass, _, err := reEncryptNodePassphrase(photoLink, rootKR, albumKR, dc.AddrKR)
		if err != nil {
			return fmt.Errorf("re-encrypt passphrase %s: %w", pid, err)
		}
		item := map[string]any{
			"LinkID": pid, "Name": encName, "Hash": hash,
			"NodePassphrase": newPass, "NameSignatureEmail": dc.AddrEmail,
		}
		if ch := contentHashes[pid]; ch != "" {
			item["ContentHash"] = ch
		}
		data = append(data, item)
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums/%s/add-multiple", dc.VolumeID, albumLinkID),
		Body: map[string]any{"AlbumData": data},
	}, nil)
}

// AlbumRemovePhotos removes photos from an album (the photos themselves remain
// on the timeline).
func (s *Service) AlbumRemovePhotos(ctx context.Context, dc *Context, albumLinkID string, linkIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums/%s/remove-multiple", dc.VolumeID, albumLinkID),
		Body: map[string]any{"LinkIDs": linkIDs},
	}, nil)
}

// PhotosDelete moves photos to the trash (by volume). When permanent is set,
// the photos are then purged from the trash.
func (s *Service) PhotosDelete(ctx context.Context, dc *Context, linkIDs []string, permanent bool) error {
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/v2/volumes/%s/trash_multiple", dc.VolumeID),
		Body: map[string]any{"LinkIDs": linkIDs},
	}, nil); err != nil {
		return err
	}
	if !permanent {
		return nil
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: fmt.Sprintf("/drive/v2/volumes/%s/trash/delete_multiple", dc.VolumeID),
		Body: map[string]any{"LinkIDs": linkIDs},
	}, nil)
}

// AlbumDelete deletes an album. When deletePhotos is true the album's photos
// are trashed too; otherwise they remain on the timeline.
func (s *Service) AlbumDelete(ctx context.Context, dc *Context, albumLinkID string, deletePhotos bool) error {
	q := url.Values{}
	if deletePhotos {
		q.Set("DeleteAlbumPhotos", "1")
	} else {
		q.Set("DeleteAlbumPhotos", "0")
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "DELETE", Path: fmt.Sprintf("/drive/photos/volumes/%s/albums/%s", dc.VolumeID, albumLinkID),
		Query: q,
	}, nil)
}

// PhotoTagsRemove removes classification tags from a photo.
func (s *Service) PhotoTagsRemove(ctx context.Context, dc *Context, linkID string, tags []int) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "DELETE", Path: fmt.Sprintf("/drive/photos/volumes/%s/links/%s/tags", dc.VolumeID, linkID),
		Body: map[string]any{"Tags": tags},
	}, nil)
}
