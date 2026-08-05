package mail

import (
	"context"
	"strconv"

	"github.com/roman-16/proton-cli/internal/proton"
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
}

type rawLabel struct {
	ID, Name, Color, Path string
	Type                  int
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
		out = append(out, Label{ID: l.ID, Name: l.Name, Color: l.Color, Type: l.Type, Path: l.Path})
	}
	return out, nil
}

func (s *Service) LabelCreate(ctx context.Context, name, color string, isFolder bool, parentID string) (string, error) {
	t := labelTypeLabel
	if isFolder {
		t = labelTypeFolder
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
