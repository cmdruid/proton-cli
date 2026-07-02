package drive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/roman-16/proton-cli/internal/proton"
)

// stubDoer records the requests issued through the proton.Doer seam and replays
// a canned JSON body, so wire-format contracts can be asserted without the API
// or any crypto.
type stubDoer struct {
	reqs     []proton.Request
	respBody []byte
}

func (s *stubDoer) Do(_ context.Context, r proton.Request) (*proton.Response, error) {
	s.reqs = append(s.reqs, r)
	return &proton.Response{Status: 200, Body: s.respBody}, nil
}

func (s *stubDoer) Decode(_ context.Context, r proton.Request, out any) error {
	s.reqs = append(s.reqs, r)
	if out == nil || s.respBody == nil {
		return nil
	}
	return json.Unmarshal(s.respBody, out)
}

func (s *stubDoer) last() proton.Request { return s.reqs[len(s.reqs)-1] }

// PhotosList must set Tag=0 only when favoritesOnly is requested; a drifted
// param name or value would silently break the `--favorites` filter.
func TestPhotosListFavoritesFilterSetsTagParam(t *testing.T) {
	dc := &Context{VolumeID: "vol1"}

	t.Run("favorites only sets Tag=0", func(t *testing.T) {
		f := &stubDoer{respBody: []byte(`{"Photos":[]}`)}
		if _, err := New(f).PhotosList(context.Background(), dc, true); err != nil {
			t.Fatalf("PhotosList: %v", err)
		}
		q := f.last().Query
		if got := q.Get("Tag"); got != "0" {
			t.Errorf("Tag = %q, want %q (PhotoTag.Favorites)", got, "0")
		}
	})

	t.Run("unfiltered omits Tag", func(t *testing.T) {
		f := &stubDoer{respBody: []byte(`{"Photos":[]}`)}
		if _, err := New(f).PhotosList(context.Background(), dc, false); err != nil {
			t.Fatalf("PhotosList: %v", err)
		}
		if f.last().Query.Has("Tag") {
			t.Errorf("Tag should be absent when favoritesOnly is false, got %q", f.last().Query.Get("Tag"))
		}
	})
}

// PhotosUnfavorite must DELETE the Favorites tag (0) from each link.
func TestPhotosUnfavoriteRemovesFavoriteTag(t *testing.T) {
	f := &stubDoer{}
	dc := &Context{VolumeID: "vol1"}
	if err := New(f).PhotosUnfavorite(context.Background(), dc, []string{"link-a"}); err != nil {
		t.Fatalf("PhotosUnfavorite: %v", err)
	}
	req := f.last()
	if req.Method != "DELETE" {
		t.Errorf("method = %q, want DELETE", req.Method)
	}
	if req.Path != "/drive/photos/volumes/vol1/links/link-a/tags" {
		t.Errorf("path = %q", req.Path)
	}
	body, ok := req.Body.(map[string]any)
	if !ok {
		t.Fatalf("body is not map[string]any: %T", req.Body)
	}
	tags, ok := body["Tags"].([]int)
	if !ok || len(tags) != 1 || tags[0] != favoriteTag {
		t.Errorf("Tags = %v, want [%d] (favoriteTag)", body["Tags"], favoriteTag)
	}
}
