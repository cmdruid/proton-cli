// Package app wires the Proton services, renderer and session together for
// the CLI. One App instance per invocation.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/account/session"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/roman-16/proton-cli/internal/service/calendar"
	"github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/service/pass"
)

type Credentials struct {
	User     string
	Password string
	TOTP     string
}

type App struct {
	Profile string
	Creds   Credentials

	API *proton.Client

	Mail     *mail.Service
	Drive    *drive.Service
	Calendar *calendar.Service
	Contacts *contacts.Service
	Pass     *pass.Service

	R *render.Renderer

	DryRun  bool
	FullIDs bool

	IDCache *idcache.Cache

	// HVUnavailableDetail is a one-line diagnostic the HV resolver may stash
	// when it returns proton.ErrHVUnavailable; the cli final-error formatter
	// surfaces it.
	HVUnavailableDetail string

	mu    sync.Mutex
	cache *keys.Unlocked
}

type Options struct {
	Profile    string
	User       string
	Password   string
	TOTP       string
	APIURL     string
	AppVersion string
	Version    string
	Output     render.Format
	LogLevel   slog.Level
	Quiet      bool
	DryRun     bool
	FullIDs    bool
}

func New(opts Options) (*App, error) {
	profileName := firstNonEmpty(opts.Profile, os.Getenv("PROTON_PROFILE"), "default")

	user := firstNonEmpty(opts.User, envForProfile(profileName, "USER"))
	password := firstNonEmpty(opts.Password, envForProfile(profileName, "PASSWORD"))
	totp := firstNonEmpty(opts.TOTP, envForProfile(profileName, "TOTP"))
	apiURL := firstNonEmpty(opts.APIURL, envForProfile(profileName, "API_URL"))
	appVer := firstNonEmpty(opts.AppVersion, envForProfile(profileName, "APP_VERSION"))
	userAgent := defaultUserAgent(opts.Version)

	r := render.New(opts.Output, os.Stdout, os.Stderr, opts.LogLevel, opts.Quiet)
	c := proton.New(proton.Options{
		AppVersion: appVer, BaseURL: apiURL, Logger: r.Log, Profile: profileName, UserAgent: userAgent,
	})

	if sess, err := session.Load(profileName); err == nil && sess != nil {
		c.SetTokens(sess.UID, sess.AccessToken, sess.RefreshToken)
		c.SetEncKeyBlob(sess.EncKeyBlob)
	}

	a := &App{
		Profile:  profileName,
		Creds:    Credentials{User: user, Password: password, TOTP: totp},
		API:      c,
		Mail:     mail.New(c),
		Drive:    drive.New(c),
		Calendar: calendar.New(c),
		Contacts: contacts.New(c),
		Pass:     pass.New(c),
		R:        r,
		DryRun:   opts.DryRun,
		FullIDs:  opts.FullIDs,
		IDCache:  idcache.New(idCachePath(profileName)),
	}
	// The client persists the session file whenever its tokens change (e.g. a
	// mid-request refresh); it stays free of the persistence format by calling
	// back into saveSession, which owns the DTO assembly.
	c.SetPersistHook(func() { _ = a.saveSession() })
	return a, nil
}

// saveSession writes the current client state to the profile's session file.
func (a *App) saveSession() error {
	uid, acc, refresh := a.API.Tokens()
	return session.Save(a.Profile, session.FromParts(
		uid, acc, refresh,
		a.API.EncKeyBlob(),
		a.API.AppVersion(), a.API.BaseURL()))
}

// idCachePath mirrors the session-file convention.
func idCachePath(profile string) string {
	if profile == "" {
		profile = "default"
	}
	cd, err := os.UserConfigDir()
	if err != nil {
		cd = "."
	}
	return filepath.Join(cd, "proton-cli", "idcache", profile+".json")
}

func (a *App) Authenticate(ctx context.Context) error {
	if uid, _, _ := a.API.Tokens(); uid != "" {
		return nil
	}
	if a.Creds.User == "" {
		return fmt.Errorf("user is required (set --user or PROTON_USER)")
	}
	if a.Creds.Password == "" {
		return fmt.Errorf("password is required (set --password or PROTON_PASSWORD)")
	}
	a.R.Info(fmt.Sprintf("Authenticating as %s...", a.Creds.User))
	if err := a.API.Login(ctx, a.Creds.User, []byte(a.Creds.Password), a.Creds.TOTP); err != nil {
		return err
	}
	a.R.Success("Authenticated.")
	return a.saveSession()
}

// Unlock caches the key hierarchy after the first call.
func (a *App) Unlock(ctx context.Context) (*keys.Unlocked, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cache != nil {
		return a.cache, nil
	}
	u, err := keys.Unlock(ctx, a.API, a.Creds.Password)
	if err != nil {
		return nil, err
	}
	a.cache = u
	return u, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// defaultUserAgent honestly identifies the CLI in the User-Agent header, e.g.
// "proton-cli/1.2.3".
func defaultUserAgent(version string) string {
	if version == "" {
		version = "dev"
	}
	return "proton-cli/" + version
}
