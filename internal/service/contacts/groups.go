package contacts

import (
	"context"

	"github.com/roman-16/proton-cli/internal/errs"
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

// GroupUpdate renames or recolours a group.
//
// A group is a label to Proton, and updating one replaces the whole record
// rather than patching it - a body without a Name is refused - so the current
// one is read and the change is laid over it.
func (s *Service) GroupUpdate(ctx context.Context, id, name, color string) error {
	cur, err := s.groupByID(ctx, id)
	if err != nil {
		return err
	}
	if name != "" {
		cur.Name = name
	}
	if color != "" {
		cur.Color = color
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/core/v4/labels/" + id,
		Body: map[string]any{
			"Name": cur.Name, "Color": cur.Color,
			"Notify": cur.Notify, "Sticky": cur.Sticky,
			"Expanded": cur.Expanded, "Display": cur.Display,
		},
	}, nil)
}

// rawGroup is a contact group as Proton keeps it, with the fields its update
// replaces.
type rawGroup struct {
	ID, Name, Color                   string
	Notify, Sticky, Expanded, Display int
}

func (s *Service) groupByID(ctx context.Context, id string) (rawGroup, error) {
	var r struct{ Labels []rawGroup }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/labels", Query: proton.Query("Type", "2"),
	}, &r); err != nil {
		return rawGroup{}, err
	}
	for _, g := range r.Labels {
		if g.ID == id {
			return g, nil
		}
	}
	return rawGroup{}, &errs.NotFound{Kind: "contact group", Ref: id}
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
