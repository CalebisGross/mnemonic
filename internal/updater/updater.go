package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	githubOwner = "appsprout-dev"
	githubRepo  = "mnemonic"
	githubAPI   = "https://api.github.com"
)

// UpdateInfo holds the result of a version check against GitHub Releases.
type UpdateInfo struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
	AssetURL        string `json:"-"`
	ChecksumsURL    string `json:"-"`
}

// UpdateResult holds the result of a completed update.
type UpdateResult struct {
	PreviousVersion string `json:"previous_version"`
	NewVersion      string `json:"new_version"`
	BinaryPath      string `json:"binary_path"`
}

// githubRelease is the subset of the GitHub release API response we need.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset is the subset of the GitHub release asset API response we need.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate checks the GitHub Releases API for a newer version.
// No authentication is required for public repositories.
func CheckForUpdate(ctx context.Context, currentVersion string) (*UpdateInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPI, githubOwner, githubRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit exceeded — try again later")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release response: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// Find the asset for this platform
	assetName := fmt.Sprintf("mnemonic_%s_%s_%s%s", latestVersion, runtime.GOOS, runtime.GOARCH, archiveExt())
	var assetURL, checksumsURL string
	for _, a := range release.Assets {
		switch a.Name {
		case assetName:
			assetURL = a.BrowserDownloadURL
		case "checksums.txt":
			checksumsURL = a.BrowserDownloadURL
		}
	}

	info := &UpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: compareVersions(latestVersion, currentVersion) > 0,
		ReleaseURL:      release.HTMLURL,
		AssetURL:        assetURL,
		ChecksumsURL:    checksumsURL,
	}

	return info, nil
}

// PerformUpdate downloads and installs the update described by info.
// It downloads the archive, verifies its checksum, extracts the binary,
// and atomically replaces the current binary.
func PerformUpdate(ctx context.Context, info *UpdateInfo) (*UpdateResult, error) {
	if !info.UpdateAvailable {
		return nil, fmt.Errorf("no update available")
	}
	if info.AssetURL == "" {
		return nil, fmt.Errorf("no release asset found for %s/%s — download manually from %s", runtime.GOOS, runtime.GOARCH, info.ReleaseURL)
	}

	// Resolve the current binary path
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, fmt.Errorf("resolving symlinks: %w", err)
	}

	execDir := filepath.Dir(execPath)
	archivePath := filepath.Join(execDir, ".mnemonic.update"+archiveExt())
	newBinaryPath := filepath.Join(execDir, ".mnemonic.update.tmp")

	// Clean up temp files on failure
	defer func() {
		_ = os.Remove(archivePath)
		_ = os.Remove(newBinaryPath)
	}()

	// Download the archive
	if err := downloadFile(ctx, info.AssetURL, archivePath); err != nil {
		return nil, fmt.Errorf("downloading update: %w", err)
	}

	// Verify checksum if available
	if info.ChecksumsURL != "" {
		assetName := fmt.Sprintf("mnemonic_%s_%s_%s%s", info.LatestVersion, runtime.GOOS, runtime.GOARCH, archiveExt())
		if err := verifyChecksum(ctx, archivePath, info.ChecksumsURL, assetName); err != nil {
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract the binary from the archive
	if err := extractBinary(archivePath, newBinaryPath); err != nil {
		return nil, fmt.Errorf("extracting binary: %w", err)
	}

	// Make the new binary executable
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return nil, fmt.Errorf("setting permissions: %w", err)
	}

	// Atomic replace: rename over the current binary
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		// On permission error, give the user a helpful hint
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied replacing %s — try running with sudo, or if installed via Homebrew use: brew upgrade appsprout-dev/tap/mnemonic", execPath)
		}
		return nil, fmt.Errorf("replacing binary: %w", err)
	}

	return &UpdateResult{
		PreviousVersion: info.CurrentVersion,
		NewVersion:      info.LatestVersion,
		BinaryPath:      execPath,
	}, nil
}

// compareVersions compares two semver version strings (MAJOR.MINOR.PATCH).
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := range 3 {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseVersion splits a version string into [major, minor, patch].
// Invalid parts default to 0.
func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := range min(len(parts), 3) {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			result[i] = n
		}
	}
	return result
}

// downloadFile downloads a URL to a local file path.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", dest, err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing file: %w", err)
	}

	return f.Close()
}

// verifyChecksum downloads checksums.txt and verifies the archive's SHA256.
func verifyChecksum(ctx context.Context, archivePath, checksumsURL, expectedName string) error {
	// Download checksums.txt
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return fmt.Errorf("creating checksums request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading checksums: %w", err)
	}

	// Find the line matching our asset
	var expectedHash string
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == expectedName {
			expectedHash = fields[0]
			break
		}
	}
	if expectedHash == "" {
		return fmt.Errorf("no checksum found for %s in checksums.txt", expectedName)
	}

	// Compute SHA256 of the downloaded archive
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive for checksum: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("computing checksum: %w", err)
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// archiveExt returns the archive file extension for the current platform.
func archiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// extractBinary extracts the "mnemonic" binary from an archive.
// Supports tar.gz (macOS/Linux) and zip (Windows).
func extractBinary(archivePath, destPath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractBinaryFromZip(archivePath, destPath)
	}
	return extractBinaryFromTarGz(archivePath, destPath)
}

// extractBinaryFromZip extracts the binary from a zip archive.
func extractBinaryFromZip(archivePath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening zip archive: %w", err)
	}
	defer func() { _ = r.Close() }()

	binaryName := "mnemonic"
	if runtime.GOOS == "windows" {
		binaryName = "mnemonic.exe"
	}

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == binaryName && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("opening zip entry: %w", err)
			}
			defer func() { _ = rc.Close() }()

			out, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			// Limit copy to 500MB to prevent zip bomb attacks
			if _, err := io.Copy(out, io.LimitReader(rc, 500*1024*1024)); err != nil {
				_ = out.Close()
				return fmt.Errorf("extracting binary: %w", err)
			}
			return out.Close()
		}
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// extractBinaryFromTarGz extracts the binary from a tar.gz archive.
func extractBinaryFromTarGz(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	binaryName := "mnemonic"
	if runtime.GOOS == "windows" {
		binaryName = "mnemonic.exe"
	}

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// The binary may be at the root or in a subdirectory
		name := filepath.Base(header.Name)
		if name == binaryName && header.Typeflag == tar.TypeReg {
			out, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			// Limit copy to 500MB to prevent zip bomb attacks
			if _, err := io.Copy(out, io.LimitReader(tr, 500*1024*1024)); err != nil {
				_ = out.Close()
				return fmt.Errorf("extracting binary: %w", err)
			}
			return out.Close()
		}
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}
