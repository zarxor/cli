// Package selfupdate updates the installed Johan Bostrom CLI binary.
package selfupdate

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultReleaseBaseURL is the latest-release asset root used by the
	// bootstrap installers and published CLI binaries.
	DefaultReleaseBaseURL = "https://github.com/zarxor/cli/releases/latest/download"

	maxArchiveSize  = int64(512 << 20)
	maxBinarySize   = int64(256 << 20)
	maxChecksumSize = int64(1 << 20)
)

type archiveFormat uint8

const (
	tarGzipArchive archiveFormat = iota
	zipArchive
)

type releaseTarget struct {
	asset  string
	binary string
	format archiveFormat
}

// Options supplies network, filesystem, and replacement dependencies.
type Options struct {
	BaseURL    string
	GOOS       string
	GOARCH     string
	Executable string
	HTTPClient *http.Client
	TempDir    string
	DryRun     bool
	Progress   func(message string) error
	Replace    func(staged, destination string) (deferred bool, err error)
}

// Result describes the release asset used by a successful update.
type Result struct {
	Asset       string
	Destination string
	Deferred    bool
}

// Run downloads, verifies, and installs the latest CLI release for the host.
// DryRun performs every network, checksum, and archive validation step but
// leaves the current executable untouched.
func Run(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("update context is required")
	}

	target, err := targetFor(opts.GOOS, opts.GOARCH)
	if err != nil {
		return Result{}, err
	}
	baseURL := releaseBaseURL(opts.BaseURL)
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}

	var destination string
	if !opts.DryRun {
		destination, err = executablePath(opts.Executable)
		if err != nil {
			return Result{}, err
		}
	}

	if err := report(opts.Progress, "Preparing the CLI update…"); err != nil {
		return Result{}, err
	}
	tempDir, err := os.MkdirTemp(opts.TempDir, "jb-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("create update temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, target.asset)
	checksumPath := archivePath + ".sha256"
	if err := report(opts.Progress, "Downloading "+target.asset+"…"); err != nil {
		return Result{}, err
	}
	if err := download(ctx, client, assetURL(baseURL, target.asset), archivePath, maxArchiveSize); err != nil {
		return Result{}, err
	}

	if err := report(opts.Progress, "Downloading the release checksum…"); err != nil {
		return Result{}, err
	}
	if err := download(ctx, client, assetURL(baseURL, target.asset+".sha256"), checksumPath, maxChecksumSize); err != nil {
		return Result{}, err
	}

	if err := report(opts.Progress, "Verifying the release checksum…"); err != nil {
		return Result{}, err
	}
	expected, err := readChecksum(checksumPath, target.asset)
	if err != nil {
		return Result{}, err
	}
	actual, err := hashFile(archivePath)
	if err != nil {
		return Result{}, fmt.Errorf("hash %s: %w", target.asset, err)
	}
	if !bytes.Equal(expected, actual) {
		return Result{}, fmt.Errorf("checksum verification failed for %s", target.asset)
	}

	if err := report(opts.Progress, "Extracting "+target.binary+"…"); err != nil {
		return Result{}, err
	}
	extracted, err := extractBinary(archivePath, tempDir, target)
	if err != nil {
		return Result{}, err
	}

	if opts.DryRun {
		if err := report(opts.Progress, "Update verified; no changes made."); err != nil {
			return Result{}, err
		}
		return Result{Asset: target.asset}, nil
	}

	if err := report(opts.Progress, "Preparing the installed CLI for replacement…"); err != nil {
		return Result{}, err
	}
	staged, err := stageBinary(extracted, destination)
	if err != nil {
		return Result{}, err
	}
	deferred := false
	keepStaged := false
	defer func() {
		if !keepStaged {
			_ = os.Remove(staged)
		}
	}()

	if err := report(opts.Progress, "Replacing the installed CLI…"); err != nil {
		return Result{}, err
	}
	replace := opts.Replace
	if replace == nil {
		replace = replaceExecutable
	}
	deferred, err = replace(staged, destination)
	if err != nil {
		if deferred {
			keepStaged = true
		}
		return Result{}, err
	}
	keepStaged = deferred
	return Result{Asset: target.asset, Destination: destination, Deferred: deferred}, nil
}

func targetFor(goos, goarch string) (releaseTarget, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goos {
	case "linux":
		switch goarch {
		case "amd64", "arm64":
			return releaseTarget{
				asset:  "jb_linux_" + goarch + ".tar.gz",
				binary: "jb",
				format: tarGzipArchive,
			}, nil
		}
	case "darwin":
		switch goarch {
		case "amd64", "arm64":
			return releaseTarget{
				asset:  "jb_darwin_" + goarch + ".tar.gz",
				binary: "jb",
				format: tarGzipArchive,
			}, nil
		}
	case "windows":
		switch goarch {
		case "amd64", "arm64":
			return releaseTarget{
				asset:  "jb_windows_" + goarch + ".zip",
				binary: "jb.exe",
				format: zipArchive,
			}, nil
		}
	}
	return releaseTarget{}, fmt.Errorf("unsupported update target %s/%s", goos, goarch)
}

func releaseBaseURL(configured string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	if fromEnvironment := os.Getenv("JB_RELEASE_BASE_URL"); fromEnvironment != "" {
		return strings.TrimRight(fromEnvironment, "/")
	}
	return DefaultReleaseBaseURL
}

func assetURL(baseURL, asset string) string {
	return strings.TrimRight(baseURL, "/") + "/" + asset
}

func report(progress func(string) error, message string) error {
	if progress == nil {
		return nil
	}
	return progress(message)
}

func download(ctx context.Context, client *http.Client, url, destination string, maxSize int64) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", filepath.Base(destination), err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download %s: server returned %s", filepath.Base(destination), response.Status)
	}
	if response.ContentLength > maxSize {
		return fmt.Errorf("download %s exceeds the %d-byte limit", filepath.Base(destination), maxSize)
	}

	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create download file %s: %w", filepath.Base(destination), err)
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(response.Body, maxSize+1))
	if err != nil {
		return fmt.Errorf("write download %s: %w", filepath.Base(destination), err)
	}
	if written > maxSize {
		return fmt.Errorf("download %s exceeds the %d-byte limit", filepath.Base(destination), maxSize)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close download file %s: %w", filepath.Base(destination), err)
	}
	return nil
}

func readChecksum(path, asset string) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checksum for %s: %w", asset, err)
	}
	fields := strings.Fields(string(contents))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return nil, fmt.Errorf("invalid checksum file for %s", asset)
	}
	checksum, err := hex.DecodeString(fields[0])
	if err != nil || len(checksum) != sha256.Size {
		return nil, fmt.Errorf("invalid checksum file for %s", asset)
	}
	return checksum, nil
}

func hashFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return hash.Sum(nil), nil
}

func extractBinary(archivePath, tempDir string, target releaseTarget) (string, error) {
	switch target.format {
	case tarGzipArchive:
		return extractTarGzip(archivePath, tempDir, target.binary)
	case zipArchive:
		return extractZip(archivePath, tempDir, target.binary)
	default:
		return "", fmt.Errorf("unsupported archive format for %s", target.asset)
	}
}

func extractTarGzip(archivePath, tempDir, binary string) (string, error) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filepath.Base(archivePath), err)
	}
	defer archiveFile.Close()
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(archivePath), err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	var extracted string
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", filepath.Base(archivePath), err)
		}
		if header.Name != binary {
			continue
		}
		if extracted != "" {
			return "", fmt.Errorf("%s contains duplicate %s entries", filepath.Base(archivePath), binary)
		}
		if (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || header.Size <= 0 {
			return "", fmt.Errorf("%s contains a non-regular %s entry", filepath.Base(archivePath), binary)
		}
		if header.Size > maxBinarySize {
			return "", fmt.Errorf("%s contains a binary larger than the %d-byte limit", filepath.Base(archivePath), maxBinarySize)
		}
		extracted, err = newExtractedFile(tempDir)
		if err != nil {
			return "", err
		}
		if err := copyExtracted(extracted, tarReader, header.Size); err != nil {
			_ = os.Remove(extracted)
			return "", fmt.Errorf("extract %s: %w", binary, err)
		}
	}
	if extracted == "" {
		return "", fmt.Errorf("%s does not contain %s", filepath.Base(archivePath), binary)
	}
	return extracted, nil
}

func extractZip(archivePath, tempDir, binary string) (string, error) {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(archivePath), err)
	}
	defer archive.Close()

	var extracted string
	for _, entry := range archive.File {
		if entry.Name != binary {
			continue
		}
		if extracted != "" {
			_ = os.Remove(extracted)
			return "", fmt.Errorf("%s contains duplicate %s entries", filepath.Base(archivePath), binary)
		}
		if entry.FileInfo().IsDir() || entry.Mode()&os.ModeType != 0 {
			return "", fmt.Errorf("%s contains a non-regular %s entry", filepath.Base(archivePath), binary)
		}
		if entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > uint64(maxBinarySize) {
			return "", fmt.Errorf("%s contains a binary outside the allowed size range", filepath.Base(archivePath))
		}
		extracted, err = newExtractedFile(tempDir)
		if err != nil {
			return "", err
		}
		input, err := entry.Open()
		if err != nil {
			_ = os.Remove(extracted)
			return "", fmt.Errorf("open %s from %s: %w", binary, filepath.Base(archivePath), err)
		}
		copyErr := copyExtracted(extracted, input, int64(entry.UncompressedSize64))
		closeErr := input.Close()
		if copyErr != nil {
			_ = os.Remove(extracted)
			return "", fmt.Errorf("extract %s: %w", binary, copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(extracted)
			return "", fmt.Errorf("close %s from %s: %w", binary, filepath.Base(archivePath), closeErr)
		}
	}
	if extracted == "" {
		return "", fmt.Errorf("%s does not contain %s", filepath.Base(archivePath), binary)
	}
	return extracted, nil
}

func newExtractedFile(tempDir string) (string, error) {
	file, err := os.CreateTemp(tempDir, ".jb-update-binary-*")
	if err != nil {
		return "", fmt.Errorf("create extracted binary: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close extracted binary: %w", err)
	}
	return path, nil
}

func copyExtracted(destination string, source io.Reader, size int64) error {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, size))
	if copyErr == nil && written != size {
		copyErr = io.ErrUnexpectedEOF
	}
	if copyErr == nil {
		copyErr = file.Chmod(0o755)
	}
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func executablePath(configured string) (string, error) {
	path := configured
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("find current CLI executable: %w", err)
		}
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve current CLI executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve current CLI executable: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat current CLI executable: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("current CLI executable is a directory: %s", absPath)
	}
	return absPath, nil
}

func stageBinary(source, destination string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".update-*")
	if err != nil {
		return "", fmt.Errorf("stage the updated CLI beside %s: %w", destination, err)
	}
	staged := file.Name()
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(staged)
		}
	}()

	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open extracted CLI: %w", err)
	}
	_, copyErr := io.Copy(file, input)
	closeInputErr := input.Close()
	if copyErr != nil {
		return "", fmt.Errorf("stage the updated CLI: %w", copyErr)
	}
	if closeInputErr != nil {
		return "", fmt.Errorf("close extracted CLI: %w", closeInputErr)
	}
	if err := file.Chmod(0o755); err != nil {
		return "", fmt.Errorf("set updated CLI permissions: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync updated CLI: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close staged CLI: %w", err)
	}
	complete = true
	return staged, nil
}
