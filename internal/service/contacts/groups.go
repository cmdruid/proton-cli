package contacts

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

// Group is a contact group (a Type-2 label).
type Group struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *Service) GroupsList(ctx context.Context) ([]Group, error) {
	var r struct {
		Labels []struct{ ID, Name, Color string }
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/labels", Query: proton.Query("Type", "2")}, &r); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(r.Labels))
	for _, l := range r.Labels {
		out = append(out, Group{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	return out, nil
}

func (s *Service) GroupCreate(ctx context.Context, name, color string) (string, error) {
	var r struct{ Label struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/core/v4/labels",
		Body: map[string]any{"Name": name, "Color": color, "Type": 2},
	}, &r); err != nil {
		return "", err
	}
	return r.Label.ID, nil
}

// GroupUpdate renames or recolours a group. Only the fields given are changed,
// so a rename does not silently reset the colour.
func (s *Service) GroupUpdate(ctx context.Context, id, name, color string) error {
	body := map[string]any{}
	if name != "" {
		body["Name"] = name
	}
	if color != "" {
		body["Color"] = color
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/core/v4/labels/" + id, Body: body,
	}, nil)
}

func (s *Service) GroupDelete(ctx context.Context, id string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/core/v4/labels/" + id}, nil)
}

// GroupAdd adds contacts to a group; GroupRemove removes them. Both operate on
// whole contacts (all of a contact's emails join/leave the group).
func (s *Service) GroupAdd(ctx context.Context, groupID string, contactIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/contacts/v4/contacts/label",
		Body: map[string]any{"LabelID": groupID, "ContactIDs": contactIDs},
	}, nil)
}

func (s *Service) GroupRemove(ctx context.Context, groupID string, contactIDs []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/contacts/v4/contacts/unlabel",
		Body: map[string]any{"LabelID": groupID, "ContactIDs": contactIDs},
	}, nil)
}
