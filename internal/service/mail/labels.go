package mail

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Type  int    `json:"type"`
	Path  string `json:"path,omitempty"`
}

type rawLabel struct {
	ID, Name, Color, Path string
	Type                  int
}

func (s *Service) LabelsList(ctx context.Context) ([]Label, []Label, error) {
	var labels, folders struct{ Labels []rawLabel }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/labels", Query: proton.Query("Type", "1")}, &labels); err != nil {
		return nil, nil, err
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/labels", Query: proton.Query("Type", "3")}, &folders); err != nil {
		return nil, nil, err
	}
	toLabel := func(l rawLabel) Label {
		return Label{ID: l.ID, Name: l.Name, Color: l.Color, Type: l.Type, Path: l.Path}
	}
	var ll, ff []Label
	for _, l := range labels.Labels {
		ll = append(ll, toLabel(l))
	}
	for _, l := range folders.Labels {
		ff = append(ff, toLabel(l))
	}
	return ll, ff, nil
}

func (s *Service) LabelCreate(ctx context.Context, name, color string, isFolder bool, parentID string) (string, error) {
	t := 1
	if isFolder {
		t = 3
	}
	body := map[string]any{"Name": name, "Color": color, "Type": t}
	if parentID != "" {
		body["ParentID"] = parentID
	}
	var r struct{ Label struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/core/v4/labels", Body: body}, &r); err != nil {
		return "", err
	}
	return r.Label.ID, nil
}

// LabelUpdate changes a label/folder's name, color and/or parent. Empty fields
// are left unchanged.
func (s *Service) LabelUpdate(ctx context.Context, id, name, color, parentID string) error {
	body := map[string]any{}
	if name != "" {
		body["Name"] = name
	}
	if color != "" {
		body["Color"] = color
	}
	if parentID != "" {
		body["ParentID"] = parentID
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/core/v4/labels/" + id, Body: body}, nil)
}

func (s *Service) LabelDelete(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/core/v4/labels", Body: map[string]any{"LabelIDs": ids}}, nil)
}

type Filter struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  int    `json:"status"`
	Version int    `json:"version"`
}

func (s *Service) FiltersList(ctx context.Context) ([]Filter, error) {
	var r struct{ Filters []Filter }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/filters"}, &r); err != nil {
		return nil, err
	}
	return r.Filters, nil
}

func (s *Service) FilterCreate(ctx context.Context, name, sieve string, status int) (string, error) {
	var r struct{ Filter struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/mail/v4/filters",
		Body: map[string]any{"Name": name, "Sieve": sieve, "Version": 2, "Status": status},
	}, &r); err != nil {
		return "", err
	}
	return r.Filter.ID, nil
}

// FilterUpdate changes a filter's name and/or sieve script, preserving its
// current enabled/disabled status (use FilterEnable/FilterDisable for that).
func (s *Service) FilterUpdate(ctx context.Context, id, name, sieve string) error {
	var cur struct {
		Filter struct {
			Name, Sieve     string
			Version, Status int
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/filters/" + id}, &cur); err != nil {
		return err
	}
	if name == "" {
		name = cur.Filter.Name
	}
	if sieve == "" {
		sieve = cur.Filter.Sieve
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/filters/" + id,
		Body: map[string]any{"Name": name, "Sieve": sieve, "Version": 2, "Status": cur.Filter.Status},
	}, nil)
}

func (s *Service) FilterDelete(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/mail/v4/filters/" + id}, nil)
}
func (s *Service) FilterEnable(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/filters/" + id + "/enable"}, nil)
}
func (s *Service) FilterDisable(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/filters/" + id + "/disable"}, nil)
}

type Address struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Type        int    `json:"type"`
	Status      int    `json:"status"`
}

func (s *Service) AddressesList(ctx context.Context) ([]Address, error) {
	var r struct {
		Addresses []struct {
			ID, Email, DisplayName string
			Type, Status           int
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/addresses"}, &r); err != nil {
		return nil, err
	}
	out := make([]Address, 0, len(r.Addresses))
	for _, a := range r.Addresses {
		out = append(out, Address{ID: a.ID, Email: a.Email, DisplayName: a.DisplayName, Type: a.Type, Status: a.Status})
	}
	return out, nil
}
