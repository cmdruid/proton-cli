package mail

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

// sieveVersion is the Sieve dialect version Proton's filter endpoints expect.
const sieveVersion = 2

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
		Body: map[string]any{"Name": name, "Sieve": sieve, "Version": sieveVersion, "Status": status},
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
		Body: map[string]any{"Name": name, "Sieve": sieve, "Version": sieveVersion, "Status": cur.Filter.Status},
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
