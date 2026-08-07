package tests

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── list ──

// A table draws no header when it has no rows, so the listing needs something
// to list rather than whatever another test happened to leave behind.
func TestDriveItemsList(t *testing.T) {
	folder := "/" + testID() + "-list"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete %s", folder),
		"drive", "items", "delete", folder)

	stdout := runOK(t, "drive", "items", "list")
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, strings.TrimPrefix(folder, "/"))
}

func TestDriveItemsListJSONFieldNames(t *testing.T) {
	data := runJSONArray(t, "drive", "items", "list")
	if len(data) == 0 {
		t.Skip("drive root is empty")
	}
	item := data[0].(map[string]interface{})
	for _, field := range []string{"link_id", "name", "type", "size"} {
		if _, ok := item[field]; !ok {
			t.Errorf("expected json field %q, got keys: %v", field, keysOf(item))
		}
	}
}

// ── info ──

func TestDriveItemsInfo(t *testing.T) {
	folder := "/" + testID() + "-info"
	tmp := t.TempDir()
	src := filepath.Join(tmp, "doc.txt")
	_ = os.WriteFile(src, []byte("info-payload-12345"), 0644)

	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)
	runOK(t, "drive", "items", "upload", src, folder)

	info := runJSON(t, "drive", "items", "get", folder+"/doc.txt")
	if info["type"] != "file" {
		t.Errorf("type = %v, want file", info["type"])
	}
	if sz, _ := info["size_bytes"].(float64); sz <= 0 {
		t.Errorf("size_bytes = %v, want > 0", info["size_bytes"])
	}
	if _, ok := info["mime_type"]; !ok {
		t.Errorf("expected mime_type, got keys %v", keysOf(info))
	}
	if info["shared"] != false {
		t.Errorf("shared = %v, want false", info["shared"])
	}
	if info["signature"] != "verified" {
		t.Errorf("signature = %v, want verified (we uploaded it)", info["signature"])
	}

	// Text mode renders the key/value block.
	text := runOK(t, "drive", "items", "get", folder+"/doc.txt")
	assertContains(t, text, "Type:")
	assertContains(t, text, "Size:")
}

func TestDriveItemsInfoFolder(t *testing.T) {
	folder := "/" + testID() + "-infodir"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	info := runJSON(t, "drive", "items", "get", folder)
	if info["type"] != "folder" {
		t.Errorf("type = %v, want folder", info["type"])
	}
}

// ── upload / download lifecycle ──

func TestDriveItemsUploadDownload(t *testing.T) {
	folder := "/" + testID() + "-upload"
	tmp := t.TempDir()
	src := filepath.Join(tmp, "payload.txt")
	want := "hello from drive test"
	_ = os.WriteFile(src, []byte(want), 0644)

	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	runOK(t, "drive", "items", "upload", src, folder)
	out := filepath.Join(tmp, "out.txt")
	runOK(t, "drive", "items", "download", folder+"/payload.txt", "--output", out)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("downloaded file not readable: %v", err)
	}
	if string(data) != want {
		t.Errorf("content mismatch: got %q, want %q", string(data), want)
	}
}

func TestDriveItemsUploadFromStdin(t *testing.T) {
	folder := "/" + testID() + "-stdin"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	payload := []byte("piped payload\n")
	cmd := exec.Command(binaryPath, "drive", "items", "upload", "-", folder)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = os.Environ()
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stdin upload failed: %v\noutput: %s", err, string(b))
	}

	// Find the uploaded file (name is stdin-<ts>)
	children := runJSONArray(t, "drive", "items", "list", folder)
	if len(children) != 1 {
		t.Fatalf("expected 1 child after stdin upload, got %d", len(children))
	}
	name := children[0].(map[string]interface{})["name"].(string)
	if !strings.HasPrefix(name, "stdin-") {
		t.Errorf("expected name to start with stdin-, got %q", name)
	}

	// Download back via explicit "-" (stdout capture)
	stdout := runOK(t, "drive", "items", "download", folder+"/"+name, "--output", "-")
	if !strings.Contains(stdout, "piped payload") {
		t.Errorf("stdout download mismatch: %q", stdout)
	}
}

// A stdin DEST whose last segment isn't an existing folder names the new file:
// the basename becomes the file name and its parent is the target folder.
func TestDriveItemsUploadFromStdinNamed(t *testing.T) {
	folder := "/" + testID() + "-stdin-named"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	dest := folder + "/piped.txt"
	payload := "named piped payload\n"
	if _, stderr, code := runWithStdin(t, strings.NewReader(payload), "drive", "items", "upload", "-", dest); code != 0 {
		t.Fatalf("named stdin upload failed (exit %d): %s", code, stderr)
	}

	children := runJSONArray(t, "drive", "items", "list", folder)
	if len(children) != 1 {
		t.Fatalf("expected 1 child after named stdin upload, got %d", len(children))
	}
	if name := children[0].(map[string]interface{})["name"].(string); name != "piped.txt" {
		t.Errorf("expected name %q, got %q", "piped.txt", name)
	}

	stdout := runOK(t, "drive", "items", "download", dest, "--output", "-")
	if !strings.Contains(stdout, payload) {
		t.Errorf("stdout download mismatch: %q", stdout)
	}
}

// A stdin upload under a non-existent parent fails as not-found (exit 3) and
// names the missing folder segment, never the intended filename.
func TestDriveItemsUploadFromStdinMissingParent(t *testing.T) {
	missing := testID() + "-nope"
	_, stderr, code := runWithStdin(t, strings.NewReader("x\n"),
		"drive", "items", "upload", "-", "/"+missing+"/note.txt")
	if code != 3 {
		t.Fatalf("expected exit 3 for missing parent, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("expected error to name the missing folder %q, got: %s", missing, stderr)
	}
	if strings.Contains(stderr, "note.txt") {
		t.Errorf("error should not name the filename segment, got: %s", stderr)
	}
}

// TestDriveItemsDownloadBehaviors exercises the three download-destination
// behaviors (refuse-on-collision, --force overwrite, stdout default) against a
// single folder + uploads created once, rather than one folder+upload per
// behavior. Subtests keep per-behavior reporting.
func TestDriveItemsDownloadBehaviors(t *testing.T) {
	folder := "/" + testID() + "-dl"
	tmp := t.TempDir()
	aSrc := filepath.Join(tmp, "a.txt")
	pSrc := filepath.Join(tmp, "p.txt")
	if err := os.WriteFile(aSrc, []byte("cloud-content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pSrc, []byte("stdoutpayload"), 0644); err != nil {
		t.Fatal(err)
	}
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)
	runOK(t, "drive", "items", "upload", aSrc, folder)
	runOK(t, "drive", "items", "upload", pSrc, folder)

	t.Run("overwrite refused without --force", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out.txt")
		if err := os.WriteFile(dest, []byte("local-existing"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		_, stderr, code := run(t, "drive", "items", "download", folder+"/a.txt", "--output", dest)
		if code == 0 {
			t.Error("expected non-zero exit when destination exists without --force")
		}
		if !strings.Contains(stderr, "exists") {
			t.Errorf("expected stderr to mention 'exists', got: %s", stderr)
		}
		if data, _ := os.ReadFile(dest); string(data) != "local-existing" {
			t.Errorf("local file should be untouched, got: %q", string(data))
		}
	})

	t.Run("--force overwrites", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out.txt")
		if err := os.WriteFile(dest, []byte("local-existing"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		runOK(t, "drive", "items", "download", "--force", folder+"/a.txt", "--output", dest)
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("read after force: %v", err)
		}
		if string(data) != "cloud-content" {
			t.Errorf("--force did not overwrite, got: %q", string(data))
		}
	})

	t.Run("--output - writes to stdout", func(t *testing.T) {
		stdout := runOK(t, "drive", "items", "download", folder+"/p.txt", "--output", "-")
		assertContains(t, stdout, "stdoutpayload")
	})
}

func TestDriveItemsUploadRecursive(t *testing.T) {
	folder := "/" + testID() + "-rec"
	tmp := t.TempDir()
	tree := filepath.Join(tmp, "tree")
	_ = os.MkdirAll(filepath.Join(tree, "sub1"), 0755)
	_ = os.MkdirAll(filepath.Join(tree, "sub2", "deep"), 0755)
	_ = os.WriteFile(filepath.Join(tree, "a.txt"), []byte("A"), 0644)
	_ = os.WriteFile(filepath.Join(tree, "sub1", "b.txt"), []byte("B"), 0644)
	_ = os.WriteFile(filepath.Join(tree, "sub2", "deep", "d.txt"), []byte("D"), 0644)

	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	runOK(t, "drive", "items", "upload", "--recursive", tree, folder)

	top := runJSONArray(t, "drive", "items", "list", folder+"/tree")
	names := map[string]bool{}
	for _, c := range top {
		names[c.(map[string]interface{})["name"].(string)] = true
	}
	for _, want := range []string{"a.txt", "sub1", "sub2"} {
		if !names[want] {
			t.Errorf("expected %q in tree/, got %v", want, names)
		}
	}
	deep := runJSONArray(t, "drive", "items", "list", folder+"/tree/sub2/deep")
	if len(deep) != 1 || deep[0].(map[string]interface{})["name"].(string) != "d.txt" {
		t.Errorf("expected d.txt in tree/sub2/deep, got %v", deep)
	}
}

func TestDriveItemsUploadMultiBlock(t *testing.T) {
	folder := "/" + testID() + "-big"
	tmp := t.TempDir()
	src := filepath.Join(tmp, "big.bin")
	big := make([]byte, 8*1024*1024) // 8 MB → two 4 MB blocks
	if _, err := io.ReadFull(rand.Reader, big); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(src, big, 0644)
	hWant := sha256.Sum256(big)

	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	runOK(t, "drive", "items", "upload", src, folder)
	out := filepath.Join(tmp, "out.bin")
	runOK(t, "drive", "items", "download", folder+"/big.bin", "--output", out)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	hGot := sha256.Sum256(got)
	if hex.EncodeToString(hGot[:]) != hex.EncodeToString(hWant[:]) {
		t.Errorf("sha256 mismatch after multi-block round-trip")
	}
}

// TestDriveItemsUploadManyBlocks crosses the upload link-batch boundary (11
// blocks > the 10-per-batch request size) so the streaming uploader has to
// request links in multiple batches and upload blocks in parallel; the sha256
// round-trip proves block ordering survives the concurrency.
func TestDriveItemsUploadManyBlocks(t *testing.T) {
	folder := "/" + testID() + "-many"
	tmp := t.TempDir()
	src := filepath.Join(tmp, "many.bin")
	big := make([]byte, 11*4*1024*1024) // 44 MiB -> 11 x 4 MiB blocks
	if _, err := io.ReadFull(rand.Reader, big); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(src, big, 0644)
	hWant := sha256.Sum256(big)

	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	runOK(t, "drive", "items", "upload", src, folder)
	out := filepath.Join(tmp, "out.bin")
	runOK(t, "drive", "items", "download", folder+"/many.bin", "--output", out)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	hGot := sha256.Sum256(got)
	if hex.EncodeToString(hGot[:]) != hex.EncodeToString(hWant[:]) {
		t.Errorf("sha256 mismatch after many-block round-trip")
	}
}

// ── rename / move (re-encryption) ──

func TestDriveItemsRename(t *testing.T) {
	folder := "/" + testID() + "-rn"
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "orig.txt"), []byte("renameme"), 0644)
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)
	runOK(t, "drive", "items", "upload", filepath.Join(tmp, "orig.txt"), folder)

	runOK(t, "drive", "items", "update", "--name", "new.txt", folder+"/orig.txt")

	children := runJSONArray(t, "drive", "items", "list", folder)
	found := false
	for _, c := range children {
		if c.(map[string]interface{})["name"].(string) == "new.txt" {
			found = true
		}
	}
	if !found {
		t.Error("expected new.txt after rename")
	}

	// Decryption round-trip after rename
	out := filepath.Join(tmp, "after.txt")
	runOK(t, "drive", "items", "download", folder+"/new.txt", "--output", out)
	if b, _ := os.ReadFile(out); string(b) != "renameme" {
		t.Errorf("content mismatch after rename: %q", string(b))
	}
}

func TestDriveItemsMove(t *testing.T) {
	src := "/" + testID() + "-src"
	dst := "/" + testID() + "-dst"
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("moveme"), 0644)

	runOK(t, "drive", "folders", "create", src)
	runOK(t, "drive", "folders", "create", dst)
	cleanupRun(t, fmt.Sprintf("Delete src: proton-cli drive items delete --permanent %s", src),
		"drive", "items", "delete", src)
	cleanupRun(t, fmt.Sprintf("Delete dst: proton-cli drive items delete --permanent %s", dst),
		"drive", "items", "delete", dst)
	runOK(t, "drive", "items", "upload", filepath.Join(tmp, "f.txt"), src)

	runOK(t, "drive", "items", "move", "--into", dst, src+"/f.txt")

	children := runJSONArray(t, "drive", "items", "list", dst)
	found := false
	for _, c := range children {
		if c.(map[string]interface{})["name"].(string) == "f.txt" {
			found = true
		}
	}
	if !found {
		t.Error("expected f.txt in dst after move")
	}

	// Re-encryption round-trip after move
	out := filepath.Join(tmp, "after.txt")
	runOK(t, "drive", "items", "download", dst+"/f.txt", "--output", out)
	if b, _ := os.ReadFile(out); string(b) != "moveme" {
		t.Errorf("content mismatch after move: %q", string(b))
	}
}

// ── delete + trash ──

func TestDriveItemsDeleteAndTrashRestore(t *testing.T) {
	folder := "/" + testID() + "-trash"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Final delete: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	// Noted before the trash: a trashed item has no path any more, and its name
	// arrives encrypted, so its own ID is the only thing that still identifies
	// it. Picking the first folder in the trash instead would restore whatever
	// else happened to be in there.
	linkID, _ := runJSON(t, "drive", "items", "get", folder)["link_id"].(string)
	if linkID == "" {
		t.Fatal("drive items get should report the folder's link ID")
	}

	// Non-permanent → trash
	runOK(t, "drive", "items", "trash", folder)

	// Should appear in trash
	found := false
	for _, e := range runJSONArray(t, "drive", "trash", "list") {
		if e.(map[string]interface{})["link_id"] == linkID {
			found = true
		}
	}
	if !found {
		t.Fatal("the trashed folder should appear in the trash")
	}

	// Restore (IDs only - trashed names are encrypted)
	runOK(t, "drive", "trash", "restore", "--", linkID)

	// It should be back in root
	top := runJSONArray(t, "drive", "items", "list")
	back := false
	folderName := strings.TrimPrefix(folder, "/")
	for _, c := range top {
		if c.(map[string]interface{})["name"].(string) == folderName {
			back = true
		}
	}
	if !back {
		t.Error("restored folder should be back in root")
	}
}

// ── batch filters (all dry-run) ──

func TestDriveBatchDeletePatternDryRun(t *testing.T) {
	folder := "/" + testID() + "-pat"
	tmp := t.TempDir()
	for _, n := range []string{"a.log", "b.log", "keep.txt"} {
		_ = os.WriteFile(filepath.Join(tmp, n), []byte("x"), 0644)
	}
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)
	for _, n := range []string{"a.log", "b.log", "keep.txt"} {
		runOK(t, "drive", "items", "upload", filepath.Join(tmp, n), folder)
	}

	_, stderr := runOKStderr(t, "--dry-run", "drive", "items", "delete",
		"--pattern", "*.log", "--scope", folder, "--recursive")
	assertContains(t, stderr, "would delete 2 items")
	assertContains(t, stderr, "a.log")
	assertContains(t, stderr, "b.log")
	assertNotContains(t, stderr, "keep.txt")
}

func TestDriveBatchDeleteRequiresInput(t *testing.T) {
	_, stderr, code := run(t, "drive", "items", "delete")
	if code == 0 {
		t.Error("expected error when no PATH and no filter given")
	}
	assertContains(t, stderr, "Nothing selected")
}

// Deleting is permanent, so it is confirmed rather than assumed - and never more
// so than with --all, which covers the whole drive. A test is not a terminal, so
// the only way through is --yes, and its absence has to stop the command rather
// than hang it.
//
// This one runs without --yes on purpose. Nothing else in the suite does, and if
// the guard ever stopped working this is the test standing between a stray --all
// and the account's entire Drive.
func TestDriveBatchDeleteAllNeedsConfirming(t *testing.T) {
	_, stderr, code := run(t, "drive", "items", "delete", "--all")
	if code != 1 {
		t.Fatalf("--all alone must be stopped for confirmation, got exit %d: %s", code, stderr)
	}
	assertContains(t, stderr, "cannot be undone")
	assertContains(t, stderr, "--yes")
	assertContains(t, stderr, "--dry-run")
}

// ── folders ──

func TestDriveFoldersCreate(t *testing.T) {
	folder := "/" + testID() + "-folder"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	top := runJSONArray(t, "drive", "items", "list")
	name := strings.TrimPrefix(folder, "/")
	found := false
	for _, c := range top {
		if c.(map[string]interface{})["name"].(string) == name {
			found = true
		}
	}
	if !found {
		t.Errorf("folder %s not in root listing", folder)
	}
}

func TestDriveItemsCopy(t *testing.T) {
	base := "/" + testID() + "-copy-src"
	dest := "/" + testID() + "-copy-dst"
	runOK(t, "drive", "folders", "create", base)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", base),
		"drive", "items", "delete", base)
	runOK(t, "drive", "folders", "create", dest)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", dest),
		"drive", "items", "delete", dest)

	dir := t.TempDir()
	src := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(src, []byte("copy me"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOK(t, "drive", "items", "upload", src, base)

	runOK(t, "drive", "items", "copy", "--into", dest, base+"/f.txt")
	assertContains(t, runOK(t, "drive", "items", "list", dest), "f.txt")

	out := filepath.Join(dir, "out.txt")
	runOK(t, "drive", "items", "download", dest+"/f.txt", "--output", out)
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(got) != "copy me" {
		t.Errorf("copy content mismatch: got %q want %q", got, "copy me")
	}
}

func TestDriveItemsRevisions(t *testing.T) {
	folder := "/" + testID() + "-rev"
	runOK(t, "drive", "folders", "create", folder)
	cleanupRun(t, fmt.Sprintf("Delete folder: proton-cli drive items delete --permanent %s", folder),
		"drive", "items", "delete", folder)

	dir := t.TempDir()
	src := filepath.Join(dir, "r.txt")
	if err := os.WriteFile(src, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOK(t, "drive", "items", "upload", src, folder)

	out := runOK(t, "drive", "items", "revisions", "list", folder+"/r.txt")
	assertContains(t, out, "active")
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
}

func photoLinkIDs(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, p := range runJSONArray(t, "drive", "photos", "list") {
		if id, ok := p.(map[string]interface{})["link_id"].(string); ok {
			set[id] = true
		}
	}
	return set
}

func TestDrivePhotosWriteLifecycle(t *testing.T) {
	if _, stderr, code := run(t, "drive", "photos", "list"); code != 0 {
		if strings.Contains(stderr, "photos") {
			t.Skip("no photos share on this account")
		}
		t.Fatalf("photos list failed: %s", truncateOutput(stderr))
	}

	before := photoLinkIDs(t)

	dir := t.TempDir()
	img := filepath.Join(dir, testID()+".png")
	writePNG(t, img)
	runOK(t, "drive", "photos", "upload", img)

	// Identify the uploaded photo as the new entry in the listing.
	var photoID string
	waitFor(20*time.Second, 1*time.Second, func() bool {
		for id := range photoLinkIDs(t) {
			if !before[id] {
				photoID = id
				return true
			}
		}
		return false
	})
	if photoID == "" {
		t.Fatal("uploaded photo did not appear in the listing")
	}
	cleanupRun(t, fmt.Sprintf("Delete photo: proton-cli drive photos delete %s", photoID),
		"drive", "photos", "delete", "--", photoID)

	// Download round-trip via the unified model: explicit file, then output-dir.
	outFile := filepath.Join(dir, "photo.out")
	runOK(t, "drive", "photos", "download", "--output", outFile, photoID)
	if fi, err := os.Stat(outFile); err != nil || fi.Size() == 0 {
		t.Errorf("photos download --output produced no file: %v", err)
	}
	outDir := filepath.Join(dir, "pics")
	runOK(t, "drive", "photos", "download", "--output-dir", outDir, photoID)
	if entries, err := os.ReadDir(outDir); err != nil || len(entries) == 0 {
		t.Errorf("photos download --output-dir wrote no file: %v", err)
	}

	// Create an album; identify it as the new entry in the listing.
	albumsBefore := map[string]bool{}
	for _, a := range runJSONArray(t, "drive", "photos", "albums", "list") {
		albumsBefore[a.(map[string]interface{})["link_id"].(string)] = true
	}
	albumName := testID() + "-album"
	runOK(t, "drive", "photos", "albums", "create", "--name", albumName)
	var albumID, albumNameSeen string
	for _, a := range runJSONArray(t, "drive", "photos", "albums", "list") {
		m := a.(map[string]interface{})
		id := m["link_id"].(string)
		if !albumsBefore[id] {
			albumID = id
			albumNameSeen, _ = m["name"].(string)
		}
	}
	if albumID == "" {
		t.Fatal("created album not found in listing")
	}
	if albumNameSeen != albumName {
		t.Errorf("album name: got %q want %q", albumNameSeen, albumName)
	}
	cleanupRun(t, fmt.Sprintf("Delete album: proton-cli drive photos albums delete %s", albumID),
		"drive", "photos", "albums", "delete", "--", albumID)

	// Add the photo to the album (node-passphrase re-wrap), verify, remove.
	runOK(t, "drive", "photos", "albums", "add", albumID, photoID)
	found := false
	for _, it := range runJSONArray(t, "drive", "photos", "list", "--album", albumID) {
		if it.(map[string]interface{})["link_id"] == photoID {
			found = true
		}
	}
	if !found {
		t.Errorf("photo %s not found in album items", photoID)
	}
	runOK(t, "drive", "photos", "albums", "remove", albumID, photoID)
}

func photoInFavorites(t *testing.T, photoID string) bool {
	t.Helper()
	for _, p := range runJSONArray(t, "drive", "photos", "list", "--tag", "favorites") {
		if p.(map[string]interface{})["link_id"] == photoID {
			return true
		}
	}
	return false
}

// favoritePhotoTags returns the tag names JSON-listed for a favorited photo,
// proving tags surface as names (e.g. "favorites") rather than raw ints.
func favoritePhotoTags(t *testing.T, photoID string) []string {
	t.Helper()
	for _, p := range runJSONArray(t, "drive", "photos", "list", "--tag", "favorites") {
		m := p.(map[string]interface{})
		if m["link_id"] != photoID {
			continue
		}
		raw, _ := m["tags"].([]interface{})
		out := make([]string, 0, len(raw))
		for _, tg := range raw {
			if s, ok := tg.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// TestDrivePhotosTrash covers the D3 verb split for photos: `trash` moves a
// photo to the trash (it leaves the timeline listing), and `delete` (permanent)
// cleans it up.
func TestDrivePhotosTrash(t *testing.T) {
	if _, stderr, code := run(t, "drive", "photos", "list"); code != 0 {
		if strings.Contains(stderr, "photos") {
			t.Skip("no photos share on this account")
		}
		t.Fatalf("photos list failed: %s", truncateOutput(stderr))
	}
	before := photoLinkIDs(t)
	dir := t.TempDir()
	img := filepath.Join(dir, testID()+".png")
	writePNG(t, img)
	runOK(t, "drive", "photos", "upload", img)

	var photoID string
	waitFor(20*time.Second, 1*time.Second, func() bool {
		for id := range photoLinkIDs(t) {
			if !before[id] {
				photoID = id
				return true
			}
		}
		return false
	})
	if photoID == "" {
		t.Fatal("uploaded photo did not appear in the listing")
	}
	cleanupRun(t, fmt.Sprintf("Delete photo: proton-cli drive photos delete %s", photoID),
		"drive", "photos", "delete", "--", photoID)

	runOK(t, "drive", "photos", "trash", "--", photoID)
	if !waitFor(15*time.Second, 1*time.Second, func() bool { return !photoLinkIDs(t)[photoID] }) {
		t.Errorf("trashed photo %s still appears in the timeline listing", photoID)
	}
}

func TestDrivePhotosFavoriteRoundTrip(t *testing.T) {
	if _, stderr, code := run(t, "drive", "photos", "list"); code != 0 {
		if strings.Contains(stderr, "photos") {
			t.Skip("no photos share on this account")
		}
		t.Fatalf("photos list failed: %s", truncateOutput(stderr))
	}

	before := photoLinkIDs(t)

	dir := t.TempDir()
	img := filepath.Join(dir, testID()+".png")
	writePNG(t, img)
	runOK(t, "drive", "photos", "upload", img)

	var photoID string
	waitFor(20*time.Second, 1*time.Second, func() bool {
		for id := range photoLinkIDs(t) {
			if !before[id] {
				photoID = id
				return true
			}
		}
		return false
	})
	if photoID == "" {
		t.Fatal("uploaded photo did not appear in the listing")
	}
	cleanupRun(t, fmt.Sprintf("Delete photo: proton-cli drive photos delete %s", photoID),
		"drive", "photos", "delete", "--", photoID)

	// --dry-run must not favorite.
	_, stderr := runOKStderr(t, "--dry-run", "drive", "photos", "favorite", "--", photoID)
	assertContains(t, stderr, "Dry run")
	if photoInFavorites(t, photoID) {
		t.Error("dry-run favorite should not actually favorite the photo")
	}

	// A freshly uploaded timeline photo is favorited in place (empty body).
	runOK(t, "drive", "photos", "favorite", "--", photoID)
	if !waitFor(20*time.Second, 1*time.Second, func() bool { return photoInFavorites(t, photoID) }) {
		t.Error("photo did not appear under --tags favorites after favorite")
	}
	// Tags are surfaced by name, never as raw ints.
	tags := favoritePhotoTags(t, photoID)
	hasFav := false
	for _, tg := range tags {
		if tg == "favorites" {
			hasFav = true
		}
	}
	if !hasFav {
		t.Errorf("favorited photo tags = %v, want to contain the name \"favorites\"", tags)
	}
	// Text mode (forced TTY) also renders the tag name, never a raw int.
	ttyOut, _, _ := runWithEnv(t, map[string]string{"PROTON_CLI_FORCE_TTY": "1"}, "drive", "photos", "list", "--tag", "favorites")
	if !strings.Contains(ttyOut, "favorites") {
		t.Errorf("text-mode list --tags favorites should show the 'favorites' tag name; got:\n%s", truncateOutput(ttyOut))
	}

	// --dry-run must not unfavorite either.
	_, stderr = runOKStderr(t, "--dry-run", "drive", "photos", "unfavorite", "--", photoID)
	assertContains(t, stderr, "Dry run")
	if !photoInFavorites(t, photoID) {
		t.Error("dry-run unfavorite should not actually remove the favorite")
	}

	runOK(t, "drive", "photos", "unfavorite", "--", photoID)
	if !waitFor(20*time.Second, 1*time.Second, func() bool { return !photoInFavorites(t, photoID) }) {
		t.Error("photo still under --tags favorites after unfavorite")
	}
}

func TestDrivePhotosRead(t *testing.T) {
	_, stderr, code := run(t, "drive", "photos", "list")
	if code != 0 {
		if strings.Contains(stderr, "photos") {
			t.Skip("no photos share on this account")
		}
		t.Fatalf("photos list failed (exit %d): %s", code, truncateOutput(stderr))
	}
	// Albums listing exercises the same photos-share bootstrap + name decryption.
	runOK(t, "drive", "photos", "albums", "list")
}

func TestDrivePhotosListTags(t *testing.T) {
	// Tags are referenced by name only - an unknown name and an integer are
	// both rejected (the CLI never accepts or leaks raw enum ints).
	_, stderr, code := run(t, "drive", "photos", "list", "--tag", "bogus-"+testID())
	if code == 0 {
		t.Error("expected non-zero exit for an unknown --tags value")
	}
	assertContains(t, stderr, "--tag accepts:")
	assertContains(t, stderr, "favorites")
	if _, _, code := run(t, "drive", "photos", "list", "--tag", "2"); code == 0 {
		t.Error("expected non-zero exit for an integer --tags value (names only)")
	}

	// Filtering by a valid classification tag runs cleanly (result may be empty).
	if _, stderr, code := run(t, "drive", "photos", "list", "--tag", "videos"); code != 0 {
		if strings.Contains(stderr, "photos") {
			t.Skip("no photos share on this account")
		}
		t.Fatalf("list --tags videos failed: %s", truncateOutput(stderr))
	}
}

// TestDrivePhotosTagsSubcommandRemoved guards the deliberate scope decision:
// the web UI exposes no add/remove-tag action, so neither do we. favorite /
// unfavorite stay; the old `photos tags` subcommand is gone.
// Favouriting is a verb, not a tag to be set: `tags` would invite a second way to
// say the same thing.
func TestDrivePhotosFavouriteIsAVerb(t *testing.T) {
	help := runOK(t, "drive", "photos", "--help")
	assertContains(t, help, "favorite")
	assertContains(t, help, "unfavorite")
	for _, line := range strings.Split(help, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "tags" {
			t.Errorf("photos should not expose a 'tags' subcommand:\n%s", help)
		}
	}
}
