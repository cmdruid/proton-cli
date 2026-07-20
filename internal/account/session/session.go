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
	AppVersion string `json:"app_version"`
	BaseURL    string `json:"base_url"`
}

// FromParts assembles a Session for persistence from a client's raw state,
// keeping the transport client free of the persistence format.
func FromParts(uid, acc, refresh, encKeyBlob, appVersion, baseURL string) *Session {
	return &Session{
		UID:          uid,
		AccessToken:  acc,
		RefreshToken: refresh,
		EncKeyBlob:   encKeyBlob,
		AppVersion:   appVersion,
		BaseURL:      baseURL,
	}
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
