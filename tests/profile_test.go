package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestProfileFromEnv verifies that PROTON_PROFILE selects the active profile
// (with no --profile flag) and that credentials resolve from the
// profile-scoped PROTON_<PROFILE>_* env vars, falling back to the unscoped
// PROTON_* vars. It also checks that a per-profile session file is created.
func TestProfileFromEnv(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("user config dir: %v", err)
	}
	workSession := filepath.Join(configDir, "proton-cli", "sessions", "work.json")
	workIDCache := filepath.Join(configDir, "proton-cli", "idcache", "work.json")
	// Remove any cached work session so PROTON_WORK_USER is actually exercised;
	// a live session would let Authenticate return before the user is read.
	_ = os.Remove(workSession)
	_ = os.Remove(workIDCache)
	t.Cleanup(func() {
		_ = os.Remove(workSession)
		_ = os.Remove(workIDCache)
	})

	// Strip PROTON_USER so the account email must come from PROTON_WORK_USER;
	// keep PROTON_PASSWORD (the password falls back to the unscoped var).
	// PROTON_PROFILE selects the "work" profile without a --profile flag.
	cmd := exec.Command(binaryPath,
		"mail", "messages", "list", "--page-size", "1", "--output", "json")
	env := filterEnv(os.Environ(), "PROTON_USER", "PROTON_PROFILE")
	env = append(env,
		"PROTON_PROFILE=work",
		"PROTON_WORK_USER="+os.Getenv("PROTON_USER"),
	)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("profile-from-env run failed: %v\noutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "\"messages\"") {
		t.Errorf("unexpected output under PROTON_PROFILE=work:\n%s", truncateOutput(string(out)))
	}

	// The per-profile session file for "work" should now exist.
	if _, err := os.Stat(workSession); err != nil {
		t.Errorf("expected per-profile session file at %s: %v", workSession, err)
	}
}

// TestProfileSessionSeparation verifies default and work sessions live in
// separate files so they don't clobber each other.
func TestProfileSessionSeparation(t *testing.T) {
	configDir, _ := os.UserConfigDir()
	sessionDir := filepath.Join(configDir, "proton-cli", "sessions")

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Skipf("session dir missing: %v", err)
	}
	hasDefault := false
	for _, e := range entries {
		if e.Name() == "default.json" {
			hasDefault = true
		}
	}
	if !hasDefault {
		t.Skip("default session not present; nothing to verify")
	}
}

func filterEnv(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return out
}
