package cli

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
	"runtime"
	"strings"
	"testing"
)

func TestExecuteUpdateDryRunReportsEveryMajorStage(t *testing.T) {
	asset, archive := cliUpdateArchive(t)
	checksum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/" + asset:
			_, _ = writer.Write(archive)
		case "/" + asset + ".sha256":
			_, _ = fmt.Fprintf(writer, "%x  %s\n", checksum, asset)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("JB_RELEASE_BASE_URL", server.URL)

	var output bytes.Buffer
	if err := ExecuteWithIO(context.Background(), []string{"update", "--dry-run"}, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"Checking for the latest Johan Bostrom CLI release",
		"Downloading",
		"Verifying the release checksum",
		"Extracting",
		"Update verified; no changes made.",
		"dry-run",
	} {
		if !strings.Contains(output.String(), fragment) {
			t.Errorf("update output missing %q:\n%s", fragment, output.String())
		}
	}
}

func cliUpdateArchive(t *testing.T) (string, []byte) {
	t.Helper()
	target, err := selfupdateTargetForTest()
	if err != nil {
		t.Skip(err.Error())
	}
	binary := []byte("updated CLI binary")
	if target.format == tarGzipTestArchive {
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
		return target.asset, archive.Bytes()
	}

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
	return target.asset, archive.Bytes()
}

type cliTestTarget struct {
	asset  string
	binary string
	format uint8
}

const tarGzipTestArchive uint8 = 0

func selfupdateTargetForTest() (cliTestTarget, error) {
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			return cliTestTarget{}, fmt.Errorf("unsupported test target %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		return cliTestTarget{asset: "jb_linux_" + runtime.GOARCH + ".tar.gz", binary: "jb", format: tarGzipTestArchive}, nil
	case "windows":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			return cliTestTarget{}, fmt.Errorf("unsupported test target %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		return cliTestTarget{asset: "jb_windows_" + runtime.GOARCH + ".zip", binary: "jb.exe", format: 1}, nil
	default:
		return cliTestTarget{}, fmt.Errorf("unsupported test target %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}
