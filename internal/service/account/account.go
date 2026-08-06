// Package account reads the account itself: who it belongs to, how much space it
// uses, and what the current session is allowed to do.
//
// Plan and billing are deliberately absent. They live behind the payments
// endpoints, they are not something a script should be changing in one line, and
// the CLI's scope is what the web clients let a user do with their data.
package account

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

type Service struct {
	C proton.Doer
}

func New(c proton.Doer) *Service { return &Service{C: c} }

// Account is the account as the CLI reports it. Field names follow
// /core/v4/users, converted to the CLI's snake_case convention.
type Account struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	UsedSpace   int64  `json:"used_space"`
	MaxSpace    int64  `json:"max_space"`
	MaxUpload   int64  `json:"max_upload"`
	CreateTime  int64  `json:"create_time"`
}

// Get fetches the account record.
func (s *Service) Get(ctx context.Context) (*Account, error) {
	var r struct {
		User struct {
			ID          string
			Name        string
			Email       string
			DisplayName string
			UsedSpace   int64
			MaxSpace    int64
			MaxUpload   int64
			CreateTime  int64
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/users"}, &r); err != nil {
		return nil, err
	}
	u := r.User
	// Proton leaves Email empty on some accounts and carries the username in
	// Name instead; the address is what a person recognises, so prefer it and
	// fall back rather than showing nothing.
	email := u.Email
	if email == "" {
		email = u.Name
	}
	name := u.DisplayName
	if name == "" {
		name = u.Name
	}
	return &Account{
		ID: u.ID, Email: email, DisplayName: name,
		UsedSpace: u.UsedSpace, MaxSpace: u.MaxSpace,
		MaxUpload: u.MaxUpload, CreateTime: u.CreateTime,
	}, nil
}
