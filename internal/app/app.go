// Package app wires the Proton services, renderer and session together for
// the CLI. One App instance per invocation.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/roman-16/proton-cli/internal/account/keys"
	"github.com/roman-16/proton-cli/internal/account/session"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/service/account"
	"github.com/roman-16/proton-cli/internal/service/calendar"
	"github.com/roman-16/proton-cli/internal/service/contacts"
	"github.com/roman-16/proton-cli/internal/service/drive"
	"github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/service/pass"
	"github.com/roman-16/proton-cli/internal/ui"
)

type App struct {
	Profile string
	// Creds resolves the account email, password and two-factor code, asking the
	// user only when nothing else supplies them.
	Creds *Credentials

	API *proton.Client

	Account  *account.Service
	Mail     *mail.Service
	Drive    *drive.Service
	Calendar *calendar.Service
	Contacts *contacts.Service
	Pass     *pass.Service

	// UI renders everything the command produces.
	UI *ui.UI

	DryRun  bool
	FullIDs bool
	// Yes answers every confirmation in advance, for scripts that mean it.
	Yes bool

	IDCache *idcache.Cache

	// HVUnavailableDetail is a one-line diagnostic the HV resolver may stash
	// when it returns proton.ErrHVUnavailable; the cli final-error formatter
	// surfaces it.
	HVUnavailableDetail string

	mu    sync.Mutex
	cache *keys.Unlocked

	stdinMu    sync.Mutex
	stdinClaim string

	sessionMu sync.Mutex
	userID    string
	email     string
}

// defaultProfile is the profile a command acts as when none is named.
const defaultProfile = "default"

type Options struct {
	Profile    string
	APIURL     string
	AppVersion string
	Version    string
	Output     ui.Format
	LogLevel   slog.Level
	Quiet      bool
	DryRun     bool
	FullIDs    bool
	NoColor    bool
	NoInput    bool
	Yes        bool
}

func New(opts Options) (*App, error) {
	profileName := firstNonEmpty(opts.Profile, os.Getenv("PROTON_PROFILE"), defaultProfile)

	apiURL := firstNonEmpty(opts.APIURL, os.Getenv("PROTON_API_URL"))
	appVer := firstNonEmpty(opts.AppVersion, os.Getenv("PROTON_APP_VERSION"))
	userAgent := defaultUserAgent(opts.Version)

	u := ui.New(ui.Options{
		Format:   opts.Output,
		LogLevel: opts.LogLevel,
		Quiet:    opts.Quiet,
		NoColor:  opts.NoColor,
		NoInput:  opts.NoInput,
		FullIDs:  opts.FullIDs,
	})
	c := proton.New(proton.Options{
		AppVersion: appVer, BaseURL: apiURL, Logger: u.Log, Profile: profileName, UserAgent: userAgent,
	})

	var userID, email string
	if sess, err := session.Load(profileName); err == nil && sess != nil {
		c.SetTokens(sess.UID, sess.AccessToken, sess.RefreshToken)
		c.SetEncKeyBlob(sess.EncKeyBlob)
		userID, email = sess.UserID, sess.Email
	}

	a := &App{
		Profile:  profileName,
		Creds:    newCredentials(u, email),
		API:      c,
		Account:  account.New(c),
		Mail:     mail.New(c),
		Drive:    drive.New(c),
		Calendar: calendar.New(c),
		Contacts: contacts.New(c),
		Pass:     pass.New(c),
		UI:       u,
		DryRun:   opts.DryRun,
		FullIDs:  opts.FullIDs,
		Yes:      opts.Yes,
		IDCache:  idcache.New(idCachePath(profileName)),
		userID:   userID,
		email:    email,
	}
	// The client persists the session file whenever its tokens change (e.g. a
	// mid-request refresh); it stays free of the persistence format by calling
	// back into saveSession, which owns the DTO assembly.
	c.SetPersistHook(func() { _ = a.saveSession() })
	a.Creds.stdinOwner = a.Stdin
	a.installScopeResolver()
	return a, nil
}

// Stdin hands out the process's standard input, which only one reader may have.
//
// Two things want it: --password-stdin for the account password, and `-` for a
// body, a key, or a file to upload. Whichever asked second would find an empty
// stream and fail somewhere further along with a puzzle, so it is told here
// instead, in terms of the two flags that collided.
func (a *App) Stdin(claim string) (io.Reader, error) {
	a.stdinMu.Lock()
	defer a.stdinMu.Unlock()
	if a.stdinClaim != "" {
		return nil, errs.Problemf("%s and %s both read standard input, which can only be read once.",
			a.stdinClaim, claim).
			Hint("pass the password with --password-file instead")
	}
	a.stdinClaim = claim
	return a.UI.In, nil
}

// saveSession writes the current client state to the profile's session file,
// preserving the identity fields an earlier save established. Those come from
// /core/v4/users, which the client has no business fetching, so they are set
// once by rememberIdentity and carried forward from here on.
func (a *App) saveSession() error {
	uid, acc, refresh := a.API.Tokens()
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	return session.Save(a.Profile, &session.Session{
		UID:          uid,
		AccessToken:  acc,
		RefreshToken: refresh,
		UserID:       a.userID,
		Email:        a.email,
		EncKeyBlob:   a.API.EncKeyBlob(),
		AppVersion:   a.API.AppVersion(),
		BaseURL:      a.API.BaseURL(),
	})
}

// rememberIdentity records who the session belongs to, so listing profiles can
// name each account without one API call per profile.
func (a *App) rememberIdentity(userID, email string) {
	a.sessionMu.Lock()
	a.userID, a.email = userID, email
	a.sessionMu.Unlock()
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

// SignedIn reports whether this profile holds a session.
func (a *App) SignedIn() bool {
	uid, _, _ := a.API.Tokens()
	return uid != ""
}

// Authenticate makes sure the profile is signed in.
//
// An account reaches the CLI one way: `account login` attaches it to a profile
// and saves the session. A command acts as whichever profile it was given, so
// when that profile has no session it is said here, before anything reaches the
// network.
func (a *App) Authenticate(context.Context) error {
	if a.SignedIn() {
		return nil
	}
	if a.Profile == defaultProfile {
		return errs.Problemf("You are not signed in.").
			Hint("proton-cli account login").Exit(2)
	}
	return errs.Problemf("Profile %q is not signed in.", a.Profile).
		Hint(fmt.Sprintf("proton-cli account login --profile %s", a.Profile)).Exit(2)
}

// Login attaches an account to this profile and saves the session.
//
// Signing in also unlocks the key hierarchy, which seals the key password into
// the session file. Doing both here is what makes the password a one-time cost:
// a login that only stored tokens would leave the very next command asking
// again.
//
// It is idempotent. A profile already signed in as the same account is left
// alone, so an unattended caller can run it unconditionally before its real
// work and recover by itself from a session that expired or was revoked.
//
// user is the account to attach, empty to ask for one. It is passed rather than
// resolved from the environment because this is the only place that names an
// account: everything else acts as whichever profile it was given.
func (a *App) Login(ctx context.Context, user string) error {
	if a.SignedIn() {
		if err := a.refuseRepoint(user); err != nil {
			return err
		}
		if a.resume(ctx) {
			return nil
		}
		// The saved session no longer works, so sign in again over the top of it.
	}
	if user == "" {
		var err error
		if user, err = a.Creds.User(); err != nil {
			return err
		}
	}
	password, err := a.Creds.Password("sign in")
	if err != nil {
		return err
	}
	if err := a.API.Login(ctx, user, []byte(password), a.Creds.TOTP); err != nil {
		return err
	}
	if err := a.saveSession(); err != nil {
		return err
	}
	if _, err := a.Unlock(ctx); err != nil {
		return err
	}
	return a.saveSession()
}

// refuseRepoint stops a profile being pointed at a second account behind its
// own back. Re-pointing is a fine thing to want; it just has to be said out
// loud, because the profile names the account everywhere else.
func (a *App) refuseRepoint(wanted string) error {
	if wanted == "" || a.email == "" || strings.EqualFold(wanted, a.email) {
		return nil
	}
	return errs.Problemf("Profile %q is signed in as %s.", a.Profile, a.email).
		Hint(fmt.Sprintf("proton-cli account logout --profile %s", a.Profile)).Exit(4)
}

// resume reports whether the saved session still works, unlocking it so the
// caller is left in the state a fresh sign-in would have produced.
func (a *App) resume(ctx context.Context) bool {
	if _, err := a.Account.Get(ctx); err != nil {
		return false
	}
	if _, err := a.Unlock(ctx); err != nil {
		return false
	}
	return a.saveSession() == nil
}

// Unlock returns the decrypted key hierarchy, memoised for the invocation.
//
// The password is requested lazily, and only on the path that actually needs it:
// once the session file carries the sealed key password, unlocking asks for
// nothing at all.
func (a *App) Unlock(ctx context.Context) (*keys.Unlocked, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cache != nil {
		return a.cache, nil
	}
	u, err := keys.Unlock(ctx, a.API, func() (string, error) {
		return a.Creds.Password("unlock your keys")
	})
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

// RememberIdentity records who the current session belongs to. Exported for the
// account commands, which learn it from /core/v4/users after signing in.
func (a *App) RememberIdentity(userID, email string) { a.rememberIdentity(userID, email) }

// SaveSession writes the current session state to disk.
func (a *App) SaveSession() error { return a.saveSession() }
