package version_test

import (
	"testing"

	"github.com/CTHua/hearth/internal/version"
)

func TestUnsetBuildVersionIsDev(t *testing.T) {
	if got := version.Current(); got != version.Dev {
		t.Fatalf("Current() = %q, want %q", got, version.Dev)
	}
	if !version.IsDev(version.Current()) {
		t.Fatal("IsDev(Current()) = false for an unversioned build")
	}
}

func TestParseAcceptsOnlyPlainSemver(t *testing.T) {
	valid := map[string]version.Semver{
		"0.1.0":    {Major: 0, Minor: 1, Patch: 0},
		"10.20.30": {Major: 10, Minor: 20, Patch: 30},
	}
	for input, want := range valid {
		got, err := version.Parse(input)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	invalid := []string{"", "1", "1.2", "1.2.3.4", "v1.2.3", "1.2.3-rc.1", "1.2.x", "1..3", "-1.2.3", "1.2.+3", version.Dev}
	for _, input := range invalid {
		if _, err := version.Parse(input); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", input)
		}
	}
}

func TestCompareIsNumericPerPart(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.9.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.1.2", "0.1.10", -1},
	}
	for _, c := range cases {
		a, errA := version.Parse(c.a)
		b, errB := version.Parse(c.b)
		if errA != nil || errB != nil {
			t.Fatalf("Parse: %v %v", errA, errB)
		}
		if got := version.Compare(a, b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
