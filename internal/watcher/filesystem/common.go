package filesystem

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config holds configuration for the filesystem watcher.
type Config struct {
	WatchDirs          []string
	ExcludePatterns    []string
	SensitivePatterns  []string
	MaxContentBytes    int
	MaxWatches         int // hard cap on inotify watches (Linux only, 0 = unlimited)
	ShallowDepth       int // inotify watch depth at startup (default: 3)
	PollIntervalSec    int // how often to scan cold directories (default: 45)
	PromotionThreshold int // changes in poll window to promote to hot (default: 3)
	DemotionTimeoutMin int // minutes of inactivity before demotion (default: 30)
}

// DefaultSensitivePatterns returns patterns for files that should never be ingested.
func DefaultSensitivePatterns() []string {
	return []string{
		".env",
		".env.",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
		".pem",
		".key",
		".p12",
		".pfx",
		"credentials",
		"secret",
		".keychain",
		".keystore",
		".jks",
		"known_hosts",
		"authorized_keys",
		".netrc",
		".npmrc",
		".pypirc",
		"token.json",
		"service-account",
		".htpasswd",
	}
}

// IsSensitiveFile checks if a file path matches any sensitive pattern.
// Uses filename-level matching: checks if the base filename contains the pattern.
func IsSensitiveFile(path string, patterns []string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, pattern := range patterns {
		p := strings.ToLower(pattern)
		if strings.Contains(base, p) {
			return true
		}
	}
	return false
}

// MatchesExcludePattern checks if a path matches any exclude pattern.
// Also checks with a trailing slash so that patterns like ".git/" match
// both the .git directory itself and files inside it.
func MatchesExcludePattern(path string, patterns []string) bool {
	pathWithSlash := path + "/"
	for _, pattern := range patterns {
		if strings.Contains(path, pattern) || strings.Contains(pathWithSlash, pattern) {
			return true
		}
	}
	return false
}

// IsBinaryFile checks if a file is a binary file based on extension.
func IsBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		// Executables & libraries
		".exe": true, ".bin": true, ".o": true, ".so": true,
		".dylib": true, ".a": true, ".dll": true,
		// Images
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".bmp": true, ".ico": true, ".tiff": true,
		".heic": true, ".heif": true, ".raw": true,
		// Video & audio
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".m4a": true,
		// Archives
		".zip": true, ".gz": true, ".tar": true, ".bz2": true,
		".xz": true, ".7z": true, ".rar": true, ".dmg": true,
		// Documents (binary)
		".pdf": true, ".doc": true, ".xls": true, ".ppt": true,
		".docx": true, ".xlsx": true, ".pptx": true,
		// Fonts
		".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
		// Databases
		".sqlite": true, ".sqlite-shm": true, ".sqlite-wal": true,
		".db": true, ".db-shm": true, ".db-wal": true,
		".ldb": true, ".sst": true, ".log": true,
		// Compiled / bytecode
		".class": true, ".pyc": true, ".pyo": true,
		// macOS specific
		".photoslibrary": true, ".dict": true, ".data": true,
	}
	return binaryExts[ext]
}

// IsBinaryContent checks if content appears to be binary data by looking for
// a high ratio of non-printable bytes in the first 512 bytes.
func IsBinaryContent(content string) bool {
	checkLen := len(content)
	if checkLen > 512 {
		checkLen = 512
	}
	if checkLen == 0 {
		return false
	}
	nonPrintable := 0
	for i := 0; i < checkLen; i++ {
		b := content[i]
		// Allow common text bytes: tab, newline, carriage return, and printable ASCII
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			nonPrintable++
		}
	}
	// If more than 10% of bytes are non-printable control chars, it's binary
	return float64(nonPrintable)/float64(checkLen) > 0.10
}

// ReadFileContent reads the first maxBytes of a file.
func ReadFileContent(path string, maxBytes int, log *slog.Logger) string {
	file, err := os.Open(path)
	if err != nil {
		log.Debug("failed to open file for reading", "path", path, "err", err)
		return ""
	}
	defer file.Close()

	limitedReader := io.LimitReader(file, int64(maxBytes))
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Debug("failed to read file content", "path", path, "err", err)
		return ""
	}

	// Check if file was truncated
	var nextByte [1]byte
	if n, err := file.Read(nextByte[:]); err == nil && n > 0 {
		return string(content) + "\n... [truncated]"
	}

	return string(content)
}
