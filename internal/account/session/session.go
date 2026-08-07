// Package session persists per-profile Proton auth state.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session is what one profile keeps on disk between runs.
//
// The fields mirror WebClients' DefaultPersistedSession
// (packages/shared/lib/authentication/SessionInterface.ts): the tokens, the user
// it belongs to, when it was written, and the sealed key password. Email is the
// one addition, so listing profiles does not need an API call per profile.
type Session struct {
	UID          string `json:"uid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
	// EncKeyBlob is the salted key password encrypted (AES-256-GCM) with a random
	// client key that lives server-side (PUT/GET /auth/v4/sessions/local/key).
	// Recovering the key password requires fetching that client key, so revoking
	// the session renders this blob undecryptable.
	EncKeyBlob  string `json:"enc_key_blob,omitempty"`
	PersistedAt int64  `json:"persisted_at,omitempty"`
	AppVersion  string `json:"app_version"`
	BaseURL     string `json:"base_url"`
}

// Unlocked reports whether the session can decrypt content without the account
// password, which it can once the key password has been sealed into it.
func (s *Session) Unlocked() bool { return s != nil && s.EncKeyBlob != "" }

// Profile is one named session slot on this machine.
type Profile struct {
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	Unlocked    bool   `json:"unlocked"`
	PersistedAt int64  `json:"persisted_at,omitempty"`
}

// Profiles lists the profiles that have a saved session, sorted by name.
//
// It reads the directory rather than any registry: the files are the state, so
// there is nothing that can disagree with them.
func Profiles() ([]Profile, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(d, "sessions"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		s, err := Load(name)
		if err != nil || s == nil {
			continue
		}
		out = append(out, Profile{
			Name: name, Email: s.Email,
			Unlocked: s.Unlocked(), PersistedAt: s.PersistedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Dir returns ~/.config/proton-cli.
func Dir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "proton-cli"), nil
}

// Path returns the session-file path for the given profile.
func Path(profile string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return pathIn(d, profile), nil
}

// pathIn resolves the session-file path for profile within base config dir d.
// Split out from Path so it can be tested against a temp dir.
func pathIn(d, profile string) string {
	if profile == "" {
		profile = "default"
	}
	return filepath.Join(d, "sessions", profile+".json")
}

// Load reads the session for the given profile. Returns nil (no error) when
// no session file exists yet.
func Load(profile string) (*Session, error) {
	p, err := Path(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil
	}
	if s.UID == "" || s.AccessToken == "" || s.RefreshToken == "" {
		return nil, nil
	}
	return &s, nil
}

func Save(profile string, s *Session) error {
	if profile == "" {
		profile = "default"
	}
	s.PersistedAt = time.Now().Unix()
	d, err := Dir()
	if err != nil {
		return err
	}
	newPath := filepath.Join(d, "sessions", profile+".json")
	if err := os.MkdirAll(filepath.Dir(newPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(newPath, data, 0600)
}

func Clear(profile string) error {
	p, err := Path(profile)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
