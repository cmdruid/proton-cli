package mail

import (
	"context"
	"strconv"

	"github.com/cmdruid/proton-cli/internal/errs"
	"github.com/cmdruid/proton-cli/internal/proton"
)

// Proton label types: a label tags a message, a folder contains it.
const (
	labelTypeLabel  = 1
	labelTypeFolder = 3
)

type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Type  int    `json:"type"`
	Path  string `json:"path,omitempty"`
	// Notify says whether mail landing in a folder is worth telling the reader
	// about. It is nil for a label, which is not a place mail lands: Proton
	// offers the switch on folders alone, so reporting one on a label would be
	// inventing a setting nobody can change.
	Notify *bool `json:"notify,omitempty"`
}

// Notifies reports whether this is somewhere a watch should report arrivals.
func (l Label) Notifies() bool { return l.Notify != nil && *l.Notify }

// LabelSpec is what a label or folder is made of. One type for both writes, so
// creating and updating cannot disagree about what the fields mean.
type LabelSpec struct {
	Name   string
	Color  string
	Parent string
	// Notify is nil when the caller is not saying, which on an update leaves it
	// as it was.
	Notify *bool
}

// rawLabel is a label as Proton keeps it. Updating one replaces the whole
// record, so every field it accepts is read even though the CLI only changes
// three of them.
type rawLabel struct {
	ID, Name, Color, Path, ParentID   string
	Type                              int
	Notify, Sticky, Expanded, Display int
}

func (s *Service) LabelsList(ctx context.Context) ([]Label, []Label, error) {
	labels, err := s.labelsOfType(ctx, labelTypeLabel)
	if err != nil {
		return nil, nil, err
	}
	folders, err := s.labelsOfType(ctx, labelTypeFolder)
	if err != nil {
		return nil, nil, err
	}
	return labels, folders, nil
}

func (s *Service) labelsOfType(ctx context.Context, t int) ([]Label, error) {
	var r struct{ Labels []rawLabel }
	if err := s.C.Decode(ctx, proton.Request{
		Method: "GET", Path: "/core/v4/labels",
		Query: proton.Query("Type", strconv.Itoa(t)),
	}, &r); err != nil {
		return nil, err
	}
	out := make([]Label, 0, len(r.Labels))
	for _, l := range r.Labels {
		row := Label{ID: l.ID, Name: l.Name, Color: l.Color, Type: l.Type, Path: l.Path}
		if t == labelTypeFolder {
			notify := l.Notify == 1
			row.Notify = &notify
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Service) LabelCreate(ctx context.Context, spec LabelSpec, isFolder bool) (string, error) {
	t := labelTypeLabel
	if isFolder {
		t = labelTypeFolder
	}
	body := map[string]any{"Name": spec.Name, "Color": spec.Color, "Type": t}
	if spec.Parent != "" {
		body["ParentID"] = spec.Parent
	}
	if spec.Notify != nil {
		body["Notify"] = boolInt(*spec.Notify)
	}
	var r struct{ Label struct{ ID string } }
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/core/v4/labels", Body: body}, &r); err != nil {
		return "", err
	}
	return r.Label.ID, nil
}

// LabelUpdate changes a label or folder's name, color and/or parent.
//
// Proton replaces the whole record rather than patching it - a body without a
// Name is refused - so the current one is read and the changes are laid over it.
// Everything else travels back unchanged, which is what stops a recolour
// clearing whether a folder notifies or where it sits.
func (s *Service) LabelUpdate(ctx context.Context, id string, spec LabelSpec) error {
	cur, err := s.labelByID(ctx, id)
	if err != nil {
		return err
	}
	notify := cur.Notify
	if spec.Notify != nil {
		notify = boolInt(*spec.Notify)
	}
	body := map[string]any{
		"Name":     pick(spec.Name, cur.Name),
		"Color":    pick(spec.Color, cur.Color),
		"Notify":   notify,
		"Sticky":   cur.Sticky,
		"Expanded": cur.Expanded,
		"Display":  cur.Display,
	}
	if p := pick(spec.Parent, cur.ParentID); p != "" {
		body["ParentID"] = p
	}
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/core/v4/labels/" + id, Body: body}, nil)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// labelByID finds a label or a folder. They are one resource to Proton, listed
// apart only by type.
func (s *Service) labelByID(ctx context.Context, id string) (rawLabel, error) {
	for _, t := range []int{labelTypeLabel, labelTypeFolder} {
		var r struct{ Labels []rawLabel }
		if err := s.C.Decode(ctx, proton.Request{
			Method: "GET", Path: "/core/v4/labels",
			Query: proton.Query("Type", strconv.Itoa(t)),
		}, &r); err != nil {
			return rawLabel{}, err
		}
		for _, l := range r.Labels {
			if l.ID == id {
				return l, nil
			}
		}
	}
	return rawLabel{}, &errs.NotFound{Kind: "folder or label", Ref: id}
}

// pick is the change if one was asked for, else what is already there.
func pick(want, current string) string {
	if want != "" {
		return want
	}
	return current
}

func (s *Service) LabelDelete(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{Method: "DELETE", Path: "/core/v4/labels", Body: map[string]any{"LabelIDs": ids}}, nil)
}
