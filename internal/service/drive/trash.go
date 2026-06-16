package drive

import (
	"context"

	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

// TrashEntry is a trashed link.
type TrashEntry struct {
	ShareID string `json:"share_id"`
	LinkID  string `json:"link_id"`
	Type    int    `json:"type"`
	Size    int64  `json:"size"`
	Trashed int64  `json:"trashed"`
}

// TrashList returns trashed link IDs grouped by share.
func (s *Service) TrashList(ctx context.Context, dc *Context) ([]TrashEntry, error) {
	var r struct {
		Trash []struct {
			ShareID string
			LinkIDs []string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/drive/volumes/" + dc.VolumeID + "/trash",
		Query: keys.Query("Page", "0", "PageSize", "150"),
	}, &r); err != nil {
		return nil, err
	}
	var out []TrashEntry
	for _, t := range r.Trash {
		for _, id := range t.LinkIDs {
			l, err := s.getLink(ctx, t.ShareID, id)
			if err != nil {
				out = append(out, TrashEntry{ShareID: t.ShareID, LinkID: id})
				continue
			}
			out = append(out, TrashEntry{ShareID: t.ShareID, LinkID: id, Type: l.Type, Size: l.Size})
		}
	}
	return out, nil
}

// TrashRestore restores items from trash.
func (s *Service) TrashRestore(ctx context.Context, dc *Context, linkIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/drive/v2/volumes/" + dc.VolumeID + "/trash/restore_multiple",
		Body: map[string]any{"LinkIDs": linkIDs},
	}, nil)
}

// TrashEmpty empties the trash.
func (s *Service) TrashEmpty(ctx context.Context, dc *Context) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/drive/volumes/" + dc.VolumeID + "/trash"}, nil)
}
