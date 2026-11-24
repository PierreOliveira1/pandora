package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	maxDownload    = 200 << 20
	maxExtract     = 200 << 20
	defaultBaseURL = "https://github.com"
	defaultRepo    = "PierreOliveira1/pandora"
)

type Upgrader struct {
	Client  *http.Client
	BaseURL string
	Repo    string
	GOOS    string
	GOARCH  string
}

func (u Upgrader) Latest(ctx context.Context) (string, error) {
	client := *u.httpClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	target := u.baseURL() + "/" + u.repo() + "/releases/latest"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("finding the latest pandora release: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("finding the latest pandora release: %w", err)
	}
	defer response.Body.Close()

	location := response.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("could not find the latest pandora release: HTTP %s", response.Status)
	}
	redirect, err := url.Parse(location)
	if err != nil {
		return "", errors.New("could not find the latest pandora release: invalid redirect location")
	}
	resolved := request.URL.ResolveReference(redirect)
	parts := strings.Split(strings.Trim(resolved.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "tag" {
		return "", errors.New("could not find the latest pandora release: redirect has no tag")
	}
	version := strings.TrimPrefix(parts[len(parts)-1], "v")
	if version == "" {
		return "", errors.New("could not find the latest pandora release: redirect has no version")
	}
	return version, nil
}

func Newer(candidate, current string) bool {
	candidateVersion, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	currentVersion, ok := parseVersion(current)
	if !ok {
		return true
	}
	if candidateVersion.major != currentVersion.major {
		return candidateVersion.major > currentVersion.major
	}
	if candidateVersion.minor != currentVersion.minor {
		return candidateVersion.minor > currentVersion.minor
	}
	if candidateVersion.patch != currentVersion.patch {
		return candidateVersion.patch > currentVersion.patch
	}
	if candidateVersion.prerelease == nil && currentVersion.prerelease == nil {
		return false
	}
	if candidateVersion.prerelease == nil {
		return true
	}
	if currentVersion.prerelease == nil {
		return false
	}
	return comparePrerelease(candidateVersion.prerelease, currentVersion.prerelease) > 0
}

func (u Upgrader) Install(ctx context.Context, version, exe string) error {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return errors.New("version is empty")
	}
	goos := u.goos()
	goarch := u.goarch()
	archiveName := fmt.Sprintf("pandora_%s_%s_%s", version, goos, goarch)
	binaryName := "pandora"
	if goos == "windows" {
		archiveName += ".zip"
		binaryName += ".exe"
	} else {
		archiveName += ".tar.gz"
	}
	base := u.baseURL() + "/" + u.repo() + "/releases/download/v" + version

	archive, err := u.download(ctx, base+"/"+archiveName, archiveName, version, goos, goarch)
	if err != nil {
		return err
	}
	checksums, err := u.download(ctx, base+"/checksums.txt", "checksums.txt", version, goos, goarch)
	if err != nil {
		return err
	}
	expected, ok := checksumFor(checksums, archiveName)
	if !ok {
		return fmt.Errorf("checksums.txt has no entry for %s", archiveName)
	}
	sum := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}

	binary, err := extractBinary(archive, goos, binaryName)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving the pandora executable: %w", err)
	}
	dir := filepath.Dir(resolved)
	temp, err := os.CreateTemp(dir, ".pandora-upgrade-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: reinstall with the install script or run with the needed permissions", dir)
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()

	if _, err := temp.Write(binary); err != nil {
		_ = temp.Close()
		return fmt.Errorf("writing the new pandora binary: %w", err)
	}
	if err := temp.Chmod(0o755); err != nil {
		_ = temp.Close()
		return fmt.Errorf("setting permissions on the new pandora binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing the new pandora binary: %w", err)
	}
	if err := u.replace(resolved, tempName); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func (u Upgrader) download(ctx context.Context, target, name, version, goos, goarch string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", name, err)
	}
	response, err := u.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not download %s: HTTP %s; check that v%s has a %s/%s build", name, response.Status, version, goos, goarch)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("could not download %s: %w", name, err)
	}
	if len(data) > maxDownload {
		return nil, fmt.Errorf("could not download %s: file is larger than 200 MB", name)
	}
	return data, nil
}

func extractBinary(archive []byte, goos, binaryName string) ([]byte, error) {
	if goos == "windows" {
		return extractZip(archive, binaryName)
	}
	return extractTarGz(archive, binaryName)
}

func extractTarGz(archive []byte, binaryName string) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading the release archive: %w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the release archive: %w", err)
		}
		if (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || archiveBase(header.Name) != binaryName {
			continue
		}
		if header.Size > maxExtract {
			return nil, errors.New("the pandora binary is larger than 200 MB")
		}
		return readLimited(tarReader)
	}
	return nil, fmt.Errorf("the release archive does not contain %s", binaryName)
}

func extractZip(archive []byte, binaryName string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("reading the release archive: %w", err)
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || archiveBase(file.Name) != binaryName {
			continue
		}
		if file.UncompressedSize64 > maxExtract {
			return nil, errors.New("the pandora binary is larger than 200 MB")
		}
		entry, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("reading the pandora binary: %w", err)
		}
		data, err := readLimited(entry)
		_ = entry.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("the release archive does not contain %s", binaryName)
}

func readLimited(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxExtract+1))
	if err != nil {
		return nil, fmt.Errorf("reading the pandora binary: %w", err)
	}
	if len(data) > maxExtract {
		return nil, errors.New("the pandora binary is larger than 200 MB")
	}
	return data, nil
}

func (u Upgrader) replace(exe, temp string) error {
	if u.goos() == "windows" {
		return replaceWindows(exe, temp)
	}
	if err := os.Rename(temp, exe); err != nil {
		return fmt.Errorf("replacing the pandora binary: %w", err)
	}
	return nil
}

func replaceWindows(exe, temp string) error {
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the old pandora binary: %w", err)
	}
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("replacing the pandora binary: %w", err)
	}
	if err := os.Rename(temp, exe); err != nil {
		if restoreErr := os.Rename(old, exe); restoreErr != nil {
			return fmt.Errorf("replacing the pandora binary: %w (could not restore %s: %v)", err, exe, restoreErr)
		}
		return fmt.Errorf("replacing the pandora binary: %w", err)
	}
	return nil
}

func (u Upgrader) httpClient() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

func (u Upgrader) baseURL() string {
	if u.BaseURL != "" {
		return strings.TrimRight(u.BaseURL, "/")
	}
	return defaultBaseURL
}

func (u Upgrader) repo() string {
	if u.Repo != "" {
		return u.Repo
	}
	return defaultRepo
}

func (u Upgrader) goos() string {
	if u.GOOS != "" {
		return u.GOOS
	}
	return runtime.GOOS
}

func (u Upgrader) goarch() string {
	if u.GOARCH != "" {
		return u.GOARCH
	}
	return runtime.GOARCH
}

func checksumFor(checksums []byte, archiveName string) (string, bool) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == archiveName {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func archiveBase(name string) string {
	return path.Base(strings.ReplaceAll(name, "\\", "/"))
}

type parsedVersion struct {
	major      int
	minor      int
	patch      int
	prerelease []string
}

func parseVersion(raw string) (parsedVersion, bool) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
	if raw == "" {
		return parsedVersion{}, false
	}
	if index := strings.IndexByte(raw, '+'); index >= 0 {
		raw = raw[:index]
	}
	var prerelease []string
	if index := strings.IndexByte(raw, '-'); index >= 0 {
		value := raw[index+1:]
		raw = raw[:index]
		if value == "" {
			return parsedVersion{}, false
		}
		prerelease = strings.Split(value, ".")
		for _, identifier := range prerelease {
			if identifier == "" {
				return parsedVersion{}, false
			}
		}
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}
	major, ok := parseNumber(parts[0])
	if !ok {
		return parsedVersion{}, false
	}
	minor, ok := parseNumber(parts[1])
	if !ok {
		return parsedVersion{}, false
	}
	patch, ok := parseNumber(parts[2])
	if !ok {
		return parsedVersion{}, false
	}
	return parsedVersion{major: major, minor: minor, patch: patch, prerelease: prerelease}, true
}

func parseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil
}

func comparePrerelease(left, right []string) int {
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		leftNumber, leftErr := strconv.Atoi(left[i])
		rightNumber, rightErr := strconv.Atoi(right[i])
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			return 1
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			return strings.Compare(left[i], right[i])
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}
