// Package version holds the build version and compares MAJOR.MINOR.PATCH
// version strings.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Dev is the version of a build that was not given one through ldflags.
const Dev = "dev"

// Version is set at build time with
// -ldflags "-X <module>/internal/version.Version=1.2.3".
var Version = Dev

const (
	partCount = 3
	separator = "."
)

// Semver is a parsed MAJOR.MINOR.PATCH version without prefix or pre-release.
type Semver struct {
	Major int
	Minor int
	Patch int
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

// Current returns the version this binary was built with.
func Current() string {
	return Version
}

// IsDev reports whether v is the version of an unversioned build.
func IsDev(v string) bool {
	return v == Dev
}

// Parse accepts exactly MAJOR.MINOR.PATCH with decimal, non-negative parts.
func Parse(s string) (Semver, error) {
	parts := strings.Split(s, separator)
	if len(parts) != partCount {
		return Semver{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH", s)
	}
	numbers := make([]int, partCount)
	for i, part := range parts {
		n, err := parsePart(part)
		if err != nil {
			return Semver{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH: %w", s, err)
		}
		numbers[i] = n
	}
	return Semver{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}, nil
}

func parsePart(part string) (int, error) {
	if part == "" {
		return 0, fmt.Errorf("empty part")
	}
	for _, r := range part {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("part %q is not a number", part)
		}
	}
	return strconv.Atoi(part)
}

// Compare returns -1, 0 or 1 as a is lower than, equal to or higher than b,
// comparing each part numerically.
func Compare(a, b Semver) int {
	pairs := [partCount][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}}
	for _, pair := range pairs {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
