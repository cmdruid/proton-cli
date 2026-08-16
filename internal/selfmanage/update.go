// Package selfmanage is proton's own side of GitHub: how a copy of the program
// was installed, which release is the newest, what that release changed, and how
// to replace or remove the binary in place.
//
// It backs `proton update`, `proton uninstall` and `proton changelog`, and it
// mirrors the curl install script where they overlap: the same release
// artifacts, the same SHA-256 verification.
package selfmanage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/minio/selfupdate"
	"github.com/roman-16/proton-cli/internal/progress"
	"golang.org/x/mod/semver"
)

const (
	owner = "roman-16"
	repo  = "proton-cli"
)

// releasesURL is the base of the GitHub Releases download namespace.
const releasesURL = "https://github.com/" + owner + "/" + repo + "/releases"

// changelogURL is CHANGELOG.md on the default branch.
//
// The file on main rather than the copy compiled into this binary, because the
// question worth asking is what a release the reader does not have yet changed,
// and no build can carry notes written after it.
const changelogURL = "https://raw.githubusercontent.com/" + owner + "/" + repo + "/main/CHANGELOG.md"

// Changelog fetches the project's CHANGELOG.md.
func Changelog(ctx context.Context, client *http.Client) ([]byte, error) {
	return get(ctx, client, changelogURL, nil)
}

// AssetName returns the raw release binary name for a platform, matching the
// goreleaser archive template (e.g. "proton-cli_linux_amd64",
// "proton-cli_windows_amd64.exe"). It errors for platforms with no prebuilt
// binary.
func AssetName(goos, goarch string) (string, error) {
	if goarch == "amd64" || goarch == "arm64" {
		switch goos {
		case "linux", "darwin":
			return fmt.Sprintf("%s_%s_%s", repo, goos, goarch), nil
		case "windows":
			return fmt.Sprintf("%s_windows_%s.exe", repo, goarch), nil
		}
	}
	return "", fmt.Errorf("no prebuilt binary for %s/%s", goos, goarch)
}

// LatestVersion resolves the newest published version (without a leading "v")
// by following the /releases/latest redirect to /releases/tag/vX.Y.Z. This
// avoids the GitHub API's unauthenticated rate limit.
func LatestVersion(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, releasesURL+"/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	final := resp.Request.URL.Path
	idx := strings.LastIndex(final, "/tag/")
	if idx < 0 {
		return "", fmt.Errorf("could not determine latest version from %q", resp.Request.URL)
	}
	v := strings.TrimPrefix(final[idx+len("/tag/"):], "v")
	if v == "" {
		return "", fmt.Errorf("could not determine latest version from %q", resp.Request.URL)
	}
	return v, nil
}

// IsNewer reports whether latest is a strictly newer release than current.
// A current version that is not valid semver (e.g. "dev") is treated as older,
// so a development build is always offered the latest release.
func IsNewer(latest, current string) bool {
	lv := "v" + strings.TrimPrefix(latest, "v")
	cv := "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(lv) {
		return false
	}
	if !semver.IsValid(cv) {
		return true
	}
	return semver.Compare(lv, cv) > 0
}

// ExpectedChecksum extracts the hex SHA-256 for filename from a goreleaser
// checksums.txt, whose lines are "<hex>  <filename>".
func ExpectedChecksum(checksums []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in checksums.txt", filename)
}

// Download fetches the release binary for version (with or without a leading
// "v") for the current platform, verifies it against the release's
// checksums.txt, and returns the verified bytes.
//
// The binary is tens of megabytes, so the transfer reports through prog the way
// a Drive transfer does. A nil sink is fine and reports nothing.
func Download(ctx context.Context, client *http.Client, version string, prog progress.Sink) ([]byte, error) {
	asset, err := AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	base := releasesURL + "/download/v" + strings.TrimPrefix(version, "v")

	sums, err := get(ctx, client, base+"/checksums.txt", nil)
	if err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}
	want, err := ExpectedChecksum(sums, asset)
	if err != nil {
		return nil, err
	}

	bin, err := get(ctx, client, base+"/"+asset, progress.Of(prog))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", asset, err)
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s (expected %s, got %s)", asset, want, hex.EncodeToString(got[:]))
	}
	return bin, nil
}

// Apply atomically replaces the executable at exePath with bin, rolling back on
// failure. On Windows the running binary is moved aside first.
func Apply(bin []byte, exePath string) error {
	return selfupdate.Apply(bytes.NewReader(bin), selfupdate.Options{TargetPath: exePath})
}

func get(ctx context.Context, client *http.Client, url string, prog progress.Sink) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if prog == nil {
		return io.ReadAll(resp.Body)
	}
	prog.Start(resp.ContentLength, "Downloading")
	defer prog.Done()
	return io.ReadAll(io.TeeReader(resp.Body, counter{prog}))
}

// counter turns bytes read into progress reports.
type counter struct{ sink progress.Sink }

func (c counter) Write(p []byte) (int, error) {
	c.sink.Add(int64(len(p)))
	return len(p), nil
}
