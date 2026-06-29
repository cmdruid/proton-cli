package drive

import (
	"context"
	"fmt"

	"github.com/roman-16/proton-cli/internal/proton"
)

type Revision struct {
	ID         string `json:"id"`
	State      int    `json:"state"`
	Size       int64  `json:"size"`
	CreateTime int64  `json:"create_time"`
}

func (s *Service) RevisionsList(ctx context.Context, dc *Context, path string) ([]Revision, error) {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return nil, err
	}
	if res.IsFolder {
		return nil, fmt.Errorf("%s is a folder, not a file", path)
	}
	var r struct {
		Revisions []struct {
			ID         string
			State      int
			Size       int64
			CreateTime int64
		}
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: fmt.Sprintf("/drive/shares/%s/files/%s/revisions", res.ShareID, res.LinkID),
	}, &r); err != nil {
		return nil, err
	}
	out := make([]Revision, 0, len(r.Revisions))
	for _, rev := range r.Revisions {
		out = append(out, Revision{ID: rev.ID, State: rev.State, Size: rev.Size, CreateTime: rev.CreateTime})
	}
	return out, nil
}

func (s *Service) RevisionRestore(ctx context.Context, dc *Context, path, revisionID string) error {
	res, err := s.ResolvePath(ctx, dc, path)
	if err != nil {
		return err
	}
	if res.IsFolder {
		return fmt.Errorf("%s is a folder, not a file", path)
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "POST",
		Path:   fmt.Sprintf("/drive/shares/%s/files/%s/revisions/%s/restore", res.ShareID, res.LinkID, revisionID),
	}, nil)
}
