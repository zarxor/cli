// Package release implements the interactive local release workflow.
package release

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// Version is a stable semantic version.
type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
}

// Bump identifies a semantic-version component to increment.
type Bump uint8

const (
	BumpPatch Bump = iota
	BumpMinor
	BumpMajor
)

var stablePattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ParseVersion parses a canonical stable vMAJOR.MINOR.PATCH version.
func ParseVersion(input string) (Version, error) {
	matches := stablePattern.FindStringSubmatch(input)
	if matches == nil {
		return Version{}, fmt.Errorf("version %q must match vMAJOR.MINOR.PATCH", input)
	}
	parts := [3]uint64{}
	for index := range parts {
		value, err := strconv.ParseUint(matches[index+1], 10, 64)
		if err != nil {
			return Version{}, fmt.Errorf("parse version %q: %w", input, err)
		}
		parts[index] = value
	}
	return Version{Major: parts[0], Minor: parts[1], Patch: parts[2]}, nil
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0, or 1 when v is lower than, equal to, or higher than other.
func (v Version) Compare(other Version) int {
	left := [3]uint64{v.Major, v.Minor, v.Patch}
	right := [3]uint64{other.Major, other.Minor, other.Patch}
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

// Next calculates the next stable version for bump.
func (v Version) Next(bump Bump) (Version, error) {
	switch bump {
	case BumpPatch:
		if v.Patch == math.MaxUint64 {
			return Version{}, errors.New("patch version overflow")
		}
		v.Patch++
	case BumpMinor:
		if v.Minor == math.MaxUint64 {
			return Version{}, errors.New("minor version overflow")
		}
		v.Minor++
		v.Patch = 0
	case BumpMajor:
		if v.Major == math.MaxUint64 {
			return Version{}, errors.New("major version overflow")
		}
		v.Major++
		v.Minor = 0
		v.Patch = 0
	default:
		return Version{}, fmt.Errorf("unknown version bump %d", bump)
	}
	return v, nil
}

// LatestVersion returns the greatest canonical stable tag, or v0.0.0.
func LatestVersion(tags []string) Version {
	latest := Version{}
	for _, tag := range tags {
		candidate, err := ParseVersion(tag)
		if err == nil && candidate.Compare(latest) > 0 {
			latest = candidate
		}
	}
	return latest
}
