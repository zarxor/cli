package platform

import "testing"

func TestDetectFromDebianFamilyRelease(t *testing.T) {
	got, err := DetectFrom("linux", []byte("ID=ubuntu\nID_LIKE=debian\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != Debian {
		t.Fatalf("DetectFrom() = %q, want %q", got, Debian)
	}
}

func TestDetectFromArchFamilyRelease(t *testing.T) {
	got, err := DetectFrom("linux", []byte("ID=manjaro\nID_LIKE=arch\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != Arch {
		t.Fatalf("DetectFrom() = %q, want %q", got, Arch)
	}
}

func TestDetectFromWindows(t *testing.T) {
	got, err := DetectFrom("windows", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != Windows {
		t.Fatalf("DetectFrom() = %q, want %q", got, Windows)
	}
}

func TestDetectFromRejectsUnsupportedLinux(t *testing.T) {
	_, err := DetectFrom("linux", []byte("ID=fedora\nID_LIKE=rhel fedora\n"))
	if err == nil {
		t.Fatal("DetectFrom() error = nil, want unsupported Linux error")
	}
}
