// Package session persists per-profile Proton auth state.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Session struct {
	UID          string `json:"uid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	// EncKeyBlob is the salted key password encrypted (AES-256-GCM) with a random
	// client key that lives server-side (PUT/GET /auth/v4/sessions/local/key).
	// Recovering the key password requires fetching that client key, so revoking
	// the session renders this blob undecryptable.
	EncKeyBlob string `json:"enc_key_blob,omitempty"`
	// SaltedKeyPass is the legacy cleartext field. It is read for backward
	// compatibility and migrated to EncKeyBlob on the next unlock; new code never
	// writes it.
	SaltedKeyPass string `json:"salted_key_pass,omitempty"`
	AppVersion    string `json:"app_version"`
	BaseURL       string `json:"base_url"`
}

// FromParts assembles a Session for persistence from a client's raw state. It
// prefers the encrypted key blob and keeps a legacy cleartext key password only
// until it has been migrated to a blob; it never writes new cleartext. This is
// the sole place that owns the "which key field wins" rule, so the transport
// client stays free of the persistence format.
func FromParts(uid, acc, refresh, encKeyBlob, saltedKeyPass, appVersion, baseURL string) *Session {
	s := &Session{
		UID:          uid,
		AccessToken:  acc,
		RefreshToken: refresh,
		AppVersion:   appVersion,
		BaseURL:      baseURL,
	}
	switch {
	case encKeyBlob != "":
		s.EncKeyBlob = encKeyBlob
	case saltedKeyPass != "":
		s.SaltedKeyPass = saltedKeyPass
	}
	return s
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
// Default profile falls back to the legacy ~/.config/proton-cli/session.json
// if present (transparent migration).
func Path(profile string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return pathIn(d, profile), nil
}

// pathIn resolves the session-file path for profile within base config dir d.
// For the default profile it prefers an existing new-style file, then a legacy
// ~/.config/proton-cli/session.json, else the new-style path. Split out from
// Path so the legacy-migration branch can be tested against a temp dir.
func pathIn(d, profile string) string {
	if profile == "" {
		profile = "default"
	}
	newPath := filepath.Join(d, "sessions", profile+".json")
	if profile == "default" {
		if _, err := os.Stat(newPath); err == nil {
			return newPath
		}
		legacy := filepath.Join(d, "session.json")
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return newPath
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
