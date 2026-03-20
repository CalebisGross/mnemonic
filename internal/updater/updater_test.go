package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"0.13.0", "0.12.0", 1},
		{"0.13.0", "0.13.0", 0},
		{"0.13.0", "0.14.0", -1},
		{"1.0.0", "0.99.99", 1},
		// With v prefix
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		// Partial versions
		{"1.0", "1.0.0", 0},
		{"1", "1.0.0", 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.a, tt.b), func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"0.13.0", [3]int{0, 13, 0}},
		{"1.0", [3]int{1, 0, 0}},
		{"1", [3]int{1, 0, 0}},
		{"dev", [3]int{0, 0, 0}},
		{"", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckForUpdate(t *testing.T) {
	// Create a mock GitHub API server
	release := githubRelease{
		TagName: "v0.14.0",
		HTMLURL: "https://github.com/appsprout-dev/mnemonic/releases/tag/v0.14.0",
		Assets: []githubAsset{
			{
				Name:               fmt.Sprintf("mnemonic_0.14.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH),
				BrowserDownloadURL: "https://example.com/mnemonic.tar.gz",
			},
			{
				Name:               "checksums.txt",
				BrowserDownloadURL: "https://example.com/checksums.txt",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// Temporarily override the GitHub API URL by testing via the exported function
	// We need to test the parsing logic, so we'll use a custom approach
	t.Run("update_available", func(t *testing.T) {
		info, err := checkForUpdateFromURL(context.Background(), "0.13.0", server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !info.UpdateAvailable {
			t.Error("expected update to be available")
		}
		if info.LatestVersion != "0.14.0" {
			t.Errorf("expected latest version 0.14.0, got %s", info.LatestVersion)
		}
		if info.AssetURL == "" {
			t.Error("expected asset URL to be set")
		}
		if info.ChecksumsURL == "" {
			t.Error("expected checksums URL to be set")
		}
	})

	t.Run("already_up_to_date", func(t *testing.T) {
		info, err := checkForUpdateFromURL(context.Background(), "0.14.0", server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.UpdateAvailable {
			t.Error("expected no update available")
		}
	})

	t.Run("newer_than_latest", func(t *testing.T) {
		info, err := checkForUpdateFromURL(context.Background(), "0.15.0", server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.UpdateAvailable {
			t.Error("expected no update available when running newer version")
		}
	})
}

func TestCheckForUpdateRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := checkForUpdateFromURL(context.Background(), "0.13.0", server.URL)
	if err == nil {
		t.Fatal("expected error for rate-limited response")
	}
	if got := err.Error(); got != "GitHub API rate limit exceeded — try again later" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestExtractBinary(t *testing.T) {
	// Create a test tar.gz archive containing a fake "mnemonic" binary
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	destPath := filepath.Join(tmpDir, "mnemonic_extracted")
	binaryContent := []byte("#!/bin/sh\necho hello\n")

	// Build the archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	binaryName := "mnemonic"
	if runtime.GOOS == "windows" {
		binaryName = "mnemonic.exe"
	}

	// Add a non-binary file first (e.g. README)
	if err := tw.WriteHeader(&tar.Header{Name: "README.md", Size: 5, Mode: 0644, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	// Add the binary
	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Size: int64(len(binaryContent)), Mode: 0755, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Extract
	if err := extractBinary(archivePath, destPath); err != nil {
		t.Fatalf("extractBinary failed: %v", err)
	}

	// Verify content
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", got, binaryContent)
	}
}

func TestExtractBinaryNotFound(t *testing.T) {
	// Create an archive without the mnemonic binary
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	if err := tw.WriteHeader(&tar.Header{Name: "README.md", Size: 5, Mode: 0644, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = extractBinary(archivePath, filepath.Join(tmpDir, "out"))
	if err == nil {
		t.Fatal("expected error when binary not in archive")
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")
	destPath := filepath.Join(tmpDir, "mnemonic_extracted")
	binaryContent := []byte("#!/bin/sh\necho hello\n")

	binaryName := "mnemonic"
	if runtime.GOOS == "windows" {
		binaryName = "mnemonic.exe"
	}

	// Build the zip archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)

	// Add a non-binary file first
	w, err := zw.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	// Add the binary
	w, err = zw.Create(binaryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(binaryContent); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Extract
	if err := extractBinary(archivePath, destPath); err != nil {
		t.Fatalf("extractBinary (zip) failed: %v", err)
	}

	// Verify content
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", got, binaryContent)
	}
}

func TestExtractBinaryFromZipNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)

	w, err := zw.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = extractBinary(archivePath, filepath.Join(tmpDir, "out"))
	if err == nil {
		t.Fatal("expected error when binary not in zip archive")
	}
}

func TestVerifyChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	testContent := []byte("test archive content")

	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute expected hash
	h := sha256.Sum256(testContent)
	expectedHash := fmt.Sprintf("%x", h)

	// Create a mock checksums server
	checksumContent := fmt.Sprintf("%s  test.tar.gz\n%s  other.tar.gz\n", expectedHash, "deadbeef")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, checksumContent)
	}))
	defer server.Close()

	t.Run("valid_checksum", func(t *testing.T) {
		err := verifyChecksum(context.Background(), testFile, server.URL, "test.tar.gz")
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("wrong_filename", func(t *testing.T) {
		err := verifyChecksum(context.Background(), testFile, server.URL, "nonexistent.tar.gz")
		if err == nil {
			t.Error("expected error for missing checksum entry")
		}
	})

	t.Run("checksum_mismatch", func(t *testing.T) {
		// Write different content to the file
		if err := os.WriteFile(testFile, []byte("different content"), 0644); err != nil {
			t.Fatal(err)
		}
		err := verifyChecksum(context.Background(), testFile, server.URL, "test.tar.gz")
		if err == nil {
			t.Error("expected error for checksum mismatch")
		}
	})
}

// checkForUpdateFromURL is a test helper that allows overriding the GitHub API URL.
func checkForUpdateFromURL(ctx context.Context, currentVersion, apiURL string) (*UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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

	latestVersion := release.TagName
	if len(latestVersion) > 0 && latestVersion[0] == 'v' {
		latestVersion = latestVersion[1:]
	}

	assetName := fmt.Sprintf("mnemonic_%s_%s_%s.tar.gz", latestVersion, runtime.GOOS, runtime.GOARCH)
	var assetURL, checksumsURL string
	for _, a := range release.Assets {
		switch a.Name {
		case assetName:
			assetURL = a.BrowserDownloadURL
		case "checksums.txt":
			checksumsURL = a.BrowserDownloadURL
		}
	}

	return &UpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: compareVersions(latestVersion, currentVersion) > 0,
		ReleaseURL:      release.HTMLURL,
		AssetURL:        assetURL,
		ChecksumsURL:    checksumsURL,
	}, nil
}
