package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMissing(t *testing.T) {
	c, err := loadFile(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("loadFile on missing file should not error: %v", err)
	}
	if c.DefaultProfile != "default" {
		t.Errorf("DefaultProfile = %q, want default", c.DefaultProfile)
	}
	if c.Profiles == nil {
		t.Error("Profiles should be non-nil")
	}
}

func TestLoadFileParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `default_profile = "work"

[profiles.work]
user = "alice@company.com"
api_url = "https://mail.proton.me/api"

[profiles.personal]
user = "alice@proton.me"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if c.DefaultProfile != "work" {
		t.Errorf("DefaultProfile = %q, want work", c.DefaultProfile)
	}
	if c.Profiles["work"].User != "alice@company.com" {
		t.Errorf("work.User = %q", c.Profiles["work"].User)
	}
	if c.Profiles["work"].APIURL != "https://mail.proton.me/api" {
		t.Errorf("work.APIURL = %q", c.Profiles["work"].APIURL)
	}
	if c.Profiles["personal"].User != "alice@proton.me" {
		t.Errorf("personal.User = %q", c.Profiles["personal"].User)
	}
}

func TestLoadFileDefaultsProfileName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[profiles.x]\nuser = \"u\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.DefaultProfile != "default" {
		t.Errorf("missing default_profile should default to \"default\", got %q", c.DefaultProfile)
	}
}

func TestLoadFileMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("this is = = not toml ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFile(path); err == nil {
		t.Error("malformed TOML should error")
	}
}

func TestResolve(t *testing.T) {
	c := &Config{
		DefaultProfile: "work",
		Profiles: map[string]Profile{
			"work":     {User: "w@x.com"},
			"personal": {User: "p@x.com"},
		},
	}

	t.Run("empty name uses default profile", func(t *testing.T) {
		name, p := c.Resolve("")
		if name != "work" || p.User != "w@x.com" {
			t.Errorf("Resolve(\"\") = %q,%+v", name, p)
		}
	})
	t.Run("named profile", func(t *testing.T) {
		name, p := c.Resolve("personal")
		if name != "personal" || p.User != "p@x.com" {
			t.Errorf("Resolve(personal) = %q,%+v", name, p)
		}
	})
	t.Run("missing profile yields empty profile but keeps name", func(t *testing.T) {
		name, p := c.Resolve("ghost")
		if name != "ghost" {
			t.Errorf("name = %q, want ghost", name)
		}
		if p.User != "" {
			t.Errorf("missing profile should be empty, got %+v", p)
		}
	})
}

func TestResolveEmptyDefaultFallsBack(t *testing.T) {
	c := &Config{Profiles: map[string]Profile{}}
	name, _ := c.Resolve("")
	if name != "default" {
		t.Errorf("empty DefaultProfile + empty arg should resolve to \"default\", got %q", name)
	}
}
