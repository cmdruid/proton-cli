package drive

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

type TrashEntry struct {
	ShareID string `json:"share_id"`
	LinkID  string `json:"link_id"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	Trashed int64  `json:"trashed"`
}

func (s *Service) TrashList(ctx context.Context, dc *Context) ([]TrashEntry, error) {
	var r struct {
		Trash []struct {
			ShareID string
			LinkIDs []string
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/drive/volumes/" + dc.VolumeID + "/trash",
		Query: proton.Query("Page", "0", "PageSize", "150"),
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
			out = append(out, TrashEntry{ShareID: t.ShareID, LinkID: id, Type: linkType(l.Type), Size: l.Size})
		}
	}
	return out, nil
}

func (s *Service) TrashRestore(ctx context.Context, dc *Context, linkIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/drive/v2/volumes/" + dc.VolumeID + "/trash/restore_multiple",
		Body: map[string]any{"LinkIDs": linkIDs},
	}, nil)
}

// TrashEmpty empties the trash of every volume the account exposes (not just
// the default one), so items trashed on secondary volumes are cleared too.
func (s *Service) TrashEmpty(ctx context.Context, dc *Context) error {
	var r struct {
		Volumes []struct{ VolumeID string }
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/drive/volumes"}, &r); err != nil {
		return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/drive/volumes/" + dc.VolumeID + "/trash"}, nil)
	}
	seen := make(map[string]bool)
	var firstErr error
	empty := func(volumeID string) {
		if volumeID == "" || seen[volumeID] {
			return
		}
		seen[volumeID] = true
		if err := s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/drive/volumes/" + volumeID + "/trash"}, nil); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, v := range r.Volumes {
		empty(v.VolumeID)
	}
	empty(dc.VolumeID)
	return firstErr
}
