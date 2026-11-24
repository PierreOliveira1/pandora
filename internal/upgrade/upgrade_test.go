package upgrade_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/upgrade"
)

func TestLatestReadsTheVersionFromTheRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/owner/repo/releases/tag/v1.2.3", http.StatusFound)
	}))
	defer server.Close()

	u := upgrade.Upgrader{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "owner/repo",
		GOOS:    "linux",
		GOARCH:  "amd64",
	}
	version, err := u.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if version != "1.2.3" {
		t.Fatalf("Latest() = %q, want 1.2.3", version)
	}
}

func TestNewerComparesSemanticVersions(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "0.3.0", current: "0.2.0", want: true},
		{candidate: "0.3.0", current: "0.3.0-rc1", want: true},
		{candidate: "0.2.10", current: "0.2.9", want: true},
		{candidate: "1.2.3", current: "1.2.3", want: false},
		{candidate: "1.2.3-rc1", current: "1.2.3", want: false},
		{candidate: "1.2.3", current: "dev", want: true},
		{candidate: "1.2.3", current: "not-a-version", want: true},
		{candidate: "dev", current: "1.2.3", want: false},
	}
	for _, test := range tests {
		if got := upgrade.Newer(test.candidate, test.current); got != test.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", test.candidate, test.current, got, test.want)
		}
	}
}

func TestInstallReplacesTheExecutableWithTheDownloadedBinary(t *testing.T) {
	archive := tarGz(t, map[string]string{
		"pandora":    "new binary",
		"README.md":  "ignored",
		"other/file": "ignored",
	})
	checksums := checksumsFor("pandora_1.2.3_linux_amd64.tar.gz", archive)
	server := releaseServer(t, "1.2.3", "linux", "amd64", archive, checksums)
	exe := fakeExe(t, "pandora", "old binary")

	u := upgrade.Upgrader{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "owner/repo",
		GOOS:    "linux",
		GOARCH:  "amd64",
	}
	if err := u.Install(context.Background(), "1.2.3", exe); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading exe: %v", err)
	}
	if string(data) != "new binary" {
		t.Fatalf("exe = %q, want the downloaded binary", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(exe)
		if err != nil {
			t.Fatalf("checking exe: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o755 {
			t.Fatalf("exe mode = %o, want 755", got)
		}
	}
	assertDirectory(t, filepath.Dir(exe), []string{"pandora"})
}

func TestInstallRefusesAChecksumMismatchAndLeavesTheExecutable(t *testing.T) {
	archive := tarGz(t, map[string]string{"pandora": "new binary"})
	checksums := "0000000000000000000000000000000000000000000000000000000000000000  pandora_1.2.3_linux_amd64.tar.gz\n"
	server := releaseServer(t, "1.2.3", "linux", "amd64", archive, checksums)
	exe := fakeExe(t, "pandora", "old binary")

	u := upgrade.Upgrader{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "owner/repo",
		GOOS:    "linux",
		GOARCH:  "amd64",
	}
	err := u.Install(context.Background(), "1.2.3", exe)
	if err == nil {
		t.Fatal("Install() accepted a bad checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Install() error = %v, want a checksum mismatch", err)
	}
	data, readErr := os.ReadFile(exe)
	if readErr != nil {
		t.Fatalf("reading exe: %v", readErr)
	}
	if string(data) != "old binary" {
		t.Fatalf("exe = %q, want the old binary", data)
	}
	assertDirectory(t, filepath.Dir(exe), []string{"pandora"})
}

func TestInstallRefusesAnArchiveWithoutTheBinary(t *testing.T) {
	archive := tarGz(t, map[string]string{"README.md": "no binary here"})
	checksums := checksumsFor("pandora_1.2.3_linux_amd64.tar.gz", archive)
	server := releaseServer(t, "1.2.3", "linux", "amd64", archive, checksums)
	exe := fakeExe(t, "pandora", "old binary")

	u := upgrade.Upgrader{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "owner/repo",
		GOOS:    "linux",
		GOARCH:  "amd64",
	}
	if err := u.Install(context.Background(), "1.2.3", exe); err == nil {
		t.Fatal("Install() accepted an archive without the binary")
	} else if !strings.Contains(err.Error(), "does not contain pandora") {
		t.Fatalf("Install() error = %v, want a missing binary error", err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading exe: %v", err)
	}
	if string(data) != "old binary" {
		t.Fatalf("exe = %q, want the old binary", data)
	}
	assertDirectory(t, filepath.Dir(exe), []string{"pandora"})
}

func TestInstallOnWindowsKeepsTheOldExecutable(t *testing.T) {
	archive := zipFile(t, map[string]string{
		"pandora.exe": "windows binary",
		"README.md":   "ignored",
	})
	checksums := checksumsFor("pandora_1.2.3_windows_amd64.zip", archive)
	server := releaseServer(t, "1.2.3", "windows", "amd64", archive, checksums)
	exe := fakeExe(t, "pandora.exe", "old binary")

	u := upgrade.Upgrader{
		Client:  server.Client(),
		BaseURL: server.URL,
		Repo:    "owner/repo",
		GOOS:    "windows",
		GOARCH:  "amd64",
	}
	if err := u.Install(context.Background(), "1.2.3", exe); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("reading exe: %v", err)
	}
	if string(data) != "windows binary" {
		t.Fatalf("exe = %q, want the downloaded binary", data)
	}
	old, err := os.ReadFile(exe + ".old")
	if err != nil {
		t.Fatalf("reading .old: %v", err)
	}
	if string(old) != "old binary" {
		t.Fatalf(".old = %q, want the old binary", old)
	}
	assertDirectory(t, filepath.Dir(exe), []string{"pandora.exe", "pandora.exe.old"})
}

func releaseServer(t *testing.T, version, goos, goarch string, archive []byte, checksums string) *httptest.Server {
	t.Helper()
	name := fmt.Sprintf("pandora_%s_%s_%s", version, goos, goarch)
	if goos == "windows" {
		name += ".zip"
	} else {
		name += ".tar.gz"
	}
	prefix := fmt.Sprintf("/owner/repo/releases/download/v%s/", version)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case prefix + name:
			_, _ = w.Write(archive)
		case prefix + "checksums.txt":
			_, _ = io.WriteString(w, checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		header := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("writing tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("writing tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("closing tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("closing gzip: %v", err)
	}
	return buffer.Bytes()
}

func zipFile(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry: %v", err)
		}
		if _, err := io.WriteString(file, content); err != nil {
			t.Fatalf("writing zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buffer.Bytes()
}

func checksumsFor(name string, archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:]) + "  " + name + "\n"
}

func fakeExe(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writing fake exe: %v", err)
	}
	return path
}

func assertDirectory(t *testing.T, dir string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading directory: %v", err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("directory = %v, want %v", names, want)
	}
}
