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

	"github.com/roman-16/proton-cli/internal/config"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/render"
	"github.com/roman-16/proton-cli/internal/service/calendar"
	"github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/session"
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
	Output     render.Format
	LogLevel   slog.Level
	Quiet      bool
	DryRun     bool
	FullIDs    bool
}

func New(opts Options) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	profileName, prof := cfg.Resolve(opts.Profile)

	user := firstNonEmpty(opts.User, os.Getenv("PROTON_USER"), prof.User)
	password := firstNonEmpty(opts.Password, os.Getenv("PROTON_PASSWORD"))
	totp := firstNonEmpty(opts.TOTP, os.Getenv("PROTON_TOTP"))
	apiURL := firstNonEmpty(opts.APIURL, os.Getenv("PROTON_API_URL"), prof.APIURL)
	appVer := firstNonEmpty(opts.AppVersion, os.Getenv("PROTON_APP_VERSION"), prof.AppVersion)

	r := render.New(opts.Output, os.Stdout, os.Stderr, opts.LogLevel, opts.Quiet)
	c := proton.New(proton.Options{
		BaseURL: apiURL, AppVersion: appVer, Profile: profileName, Logger: r.Log,
	})

	if sess, err := session.Load(profileName); err == nil && sess != nil {
		c.SetTokens(sess.UID, sess.AccessToken, sess.RefreshToken)
		c.SetEncKeyBlob(sess.EncKeyBlob)
		if sess.SaltedKeyPass != "" { // legacy file; migrated to a blob on next unlock
			c.SetSaltedKeyPass(sess.SaltedKeyPass)
		}
	}

	return &App{
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
	}, nil
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
	if a.API.Session().UID != "" {
		return nil
	}
	if a.Creds.User == "" {
		return fmt.Errorf("user is required (set --user, PROTON_USER, or configure a profile)")
	}
	if a.Creds.Password == "" {
		return fmt.Errorf("password is required (set --password or PROTON_PASSWORD)")
	}
	a.R.Info(fmt.Sprintf("Authenticating as %s...", a.Creds.User))
	if err := a.API.Login(ctx, a.Creds.User, []byte(a.Creds.Password), a.Creds.TOTP); err != nil {
		return err
	}
	a.R.Success("Authenticated.")
	return session.Save(a.Profile, a.API.Session())
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
