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

func TestDetectFromMacOS(t *testing.T) {
	got, err := DetectFrom("darwin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != Darwin {
		t.Fatalf("DetectFrom() = %q, want %q", got, Darwin)
	}
}

func TestDetectFromDirectDistributionIDs(t *testing.T) {
	tests := []struct {
		id   string
		want OS
	}{{"debian", Debian}, {"ubuntu", Debian}, {"arch", Arch}}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			got, err := DetectFrom("linux", []byte("ID="+test.id+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("DetectFrom() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetectFromRejectsUnsupportedLinux(t *testing.T) {
	_, err := DetectFrom("linux", []byte("ID=fedora\nID_LIKE=rhel fedora\n"))
	if err == nil {
		t.Fatal("DetectFrom() error = nil, want unsupported Linux error")
	}
}
