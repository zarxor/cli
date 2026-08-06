package release

import (
	"math"
	"testing"
)

func TestParseVersionAcceptsCanonicalStableVersion(t *testing.T) {
	got, err := ParseVersion("v12.3.45")
	if err != nil {
		t.Fatal(err)
	}
	want := Version{Major: 12, Minor: 3, Patch: 45}
	if got != want {
		t.Fatalf("ParseVersion() = %+v, want %+v", got, want)
	}
}

func TestParseVersionRejectsUnsupportedForms(t *testing.T) {
	for _, input := range []string{
		"1.2.3",
		"v01.2.3",
		"v1.02.3",
		"v1.2.03",
		"v1.2",
		"v1.2.3-rc.1",
		"v1.2.3+build",
		"v18446744073709551616.0.0",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseVersion(input); err == nil {
				t.Fatalf("ParseVersion(%q) succeeded", input)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name        string
		left, right Version
		want        int
	}{
		{name: "equal", left: Version{1, 2, 3}, right: Version{1, 2, 3}, want: 0},
		{name: "major", left: Version{2, 0, 0}, right: Version{1, 99, 99}, want: 1},
		{name: "minor", left: Version{1, 3, 0}, right: Version{1, 2, 99}, want: 1},
		{name: "patch", left: Version{1, 2, 3}, right: Version{1, 2, 4}, want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.left.Compare(test.right); got != test.want {
				t.Fatalf("Compare() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestVersionNext(t *testing.T) {
	current := Version{Major: 1, Minor: 2, Patch: 3}
	tests := []struct {
		name string
		bump Bump
		want string
	}{
		{name: "patch", bump: BumpPatch, want: "v1.2.4"},
		{name: "minor", bump: BumpMinor, want: "v1.3.0"},
		{name: "major", bump: BumpMajor, want: "v2.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := current.Next(test.bump)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != test.want {
				t.Fatalf("Next() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestVersionNextRejectsOverflowAndUnknownBump(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		bump    Bump
	}{
		{name: "patch overflow", version: Version{Patch: math.MaxUint64}, bump: BumpPatch},
		{name: "minor overflow", version: Version{Minor: math.MaxUint64}, bump: BumpMinor},
		{name: "major overflow", version: Version{Major: math.MaxUint64}, bump: BumpMajor},
		{name: "unknown bump", version: Version{}, bump: Bump(99)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.version.Next(test.bump); err == nil {
				t.Fatal("Next() succeeded")
			}
		})
	}
}

func TestLatestVersionIgnoresNonReleaseTags(t *testing.T) {
	got := LatestVersion([]string{"notes", "v1.2.3", "v2.0.0-rc.1", "v1.10.0"})
	if got.String() != "v1.10.0" {
		t.Fatalf("LatestVersion() = %s, want v1.10.0", got)
	}
}

func TestLatestVersionDefaultsToZero(t *testing.T) {
	if got := LatestVersion([]string{"notes", "v1.0.0-rc.1"}); got.String() != "v0.0.0" {
		t.Fatalf("LatestVersion() = %s, want v0.0.0", got)
	}
}
