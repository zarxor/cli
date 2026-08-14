package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunDownloadsVerifiesAndReplacesCLI(t *testing.T) {
	target := testTarget(t)
	binary := []byte("updated CLI binary")
	server, requests := releaseServer(t, target, binary, "")
	defer server.Close()

	destination := filepath.Join(t.TempDir(), target.binary)
	if err := os.WriteFile(destination, []byte("old CLI binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var progress []string
	result, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: destination,
		HTTPClient: server.Client(),
		TempDir:    t.TempDir(),
		Progress: func(message string) error {
			progress = append(progress, message)
			return nil
		},
		Replace: func(staged, destination string) (bool, error) {
			return false, os.Rename(staged, destination)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Asset != target.asset || result.Destination != destination || result.Deferred {
		t.Fatalf("result = %#v, want asset %q and destination %q", result, target.asset, destination)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(binary) {
		t.Fatalf("installed binary = %q, want %q", got, binary)
	}
	if got, want := *requests, []string{"/" + target.asset, "/" + target.asset + ".sha256"}; !sameStrings(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for _, message := range []string{"Downloading", "Verifying", "Extracting", "Replacing"} {
		if !containsProgress(progress, message) {
			t.Errorf("progress missing %q: %v", message, progress)
		}
	}
}

func TestRunRejectsChecksumMismatchWithoutReplacingCLI(t *testing.T) {
	target := testTarget(t)
	binary := []byte("updated CLI binary")
	server, _ := releaseServer(t, target, binary, strings.Repeat("0", sha256.Size*2))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), target.binary)
	old := []byte("old CLI binary")
	if err := os.WriteFile(destination, old, 0o755); err != nil {
		t.Fatal(err)
	}
	replaced := false
	_, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: destination,
		HTTPClient: server.Client(),
		Replace: func(staged, destination string) (bool, error) {
			replaced = true
			return false, os.Rename(staged, destination)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("Run() error = %v, want checksum failure", err)
	}
	if replaced {
		t.Fatal("replacement ran after checksum failure")
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(old) {
		t.Fatalf("destination = %q, want %q", got, old)
	}
}

func TestRunDryRunVerifiesWithoutChangingCLI(t *testing.T) {
	target := testTarget(t)
	binary := []byte("updated CLI binary")
	server, _ := releaseServer(t, target, binary, "")
	defer server.Close()

	destination := filepath.Join(t.TempDir(), target.binary)
	old := []byte("old CLI binary")
	if err := os.WriteFile(destination, old, 0o755); err != nil {
		t.Fatal(err)
	}
	replaced := false
	result, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Executable: destination,
		HTTPClient: server.Client(),
		DryRun:     true,
		Progress: func(message string) error {
			if strings.Contains(message, "no changes") && replaced {
				t.Fatal("replacement ran before dry-run completed")
			}
			return nil
		},
		Replace: func(staged, destination string) (bool, error) {
			replaced = true
			return false, os.Rename(staged, destination)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Asset != target.asset || result.Destination != "" || result.Deferred {
		t.Fatalf("dry-run result = %#v", result)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(old) {
		t.Fatalf("destination = %q, want %q", got, old)
	}
	if replaced {
		t.Fatal("replacement ran during dry-run")
	}
}

func TestTargetForSupportsMacOSReleaseTargets(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		target, err := targetFor("darwin", arch)
		if err != nil || target.asset != "jb_darwin_"+arch+".tar.gz" || target.binary != "jb" {
			t.Fatalf("targetFor(darwin/%s) = %#v, %v", arch, target, err)
		}
	}
}

func testTarget(t *testing.T) releaseTarget {
	t.Helper()
	target, err := targetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("self-update test target is unsupported on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	return target
}

func releaseServer(t *testing.T, target releaseTarget, binary []byte, checksum string) (*httptest.Server, *[]string) {
	t.Helper()
	archive := testArchive(t, target, binary)
	if checksum == "" {
		checksumBytes := sha256.Sum256(archive)
		checksum = fmt.Sprintf("%x  %s\n", checksumBytes, target.asset)
	} else {
		checksum += "  " + target.asset + "\n"
	}
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path)
		switch request.URL.Path {
		case "/" + target.asset:
			_, _ = writer.Write(archive)
		case "/" + target.asset + ".sha256":
			_, _ = io.WriteString(writer, checksum)
		default:
			http.NotFound(writer, request)
		}
	}))
	return server, &requests
}

func testArchive(t *testing.T, target releaseTarget, binary []byte) []byte {
	t.Helper()
	switch target.format {
	case tarGzipArchive:
		var tarBytes bytes.Buffer
		tarWriter := tar.NewWriter(&tarBytes)
		if err := tarWriter.WriteHeader(&tar.Header{Name: target.binary, Mode: 0o755, Size: int64(len(binary))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(binary); err != nil {
			t.Fatal(err)
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		gzipWriter := gzip.NewWriter(&archive)
		if _, err := gzipWriter.Write(tarBytes.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatal(err)
		}
		return archive.Bytes()
	case zipArchive:
		var archive bytes.Buffer
		zipWriter := zip.NewWriter(&archive)
		entry, err := zipWriter.Create(target.binary)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(binary); err != nil {
			t.Fatal(err)
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatal(err)
		}
		return archive.Bytes()
	default:
		t.Fatalf("unsupported test archive format %d", target.format)
		return nil
	}
}

func containsProgress(messages []string, fragment string) bool {
	for _, message := range messages {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
