package core

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.3.1", "v1.3.1", 0},
		{"1.3.1", "v1.3.1", 0}, // the leading v is optional
		{"v1.3", "v1.3.0", 0},  // missing components read as zero
		{"v1.3.1", "v1.3.2", -1},
		{"v1.3.9", "v1.4.0", -1},
		{"v1.9.0", "v2.0.0", -1},
		{"v1.4.0", "v1.3.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		// A release outranks its own prereleases, so an rc never reads as "newer".
		{"v1.4.0-rc1", "v1.4.0", -1},
		{"v1.4.0", "v1.4.0-rc1", 1},
		{"v1.4.0-rc1", "v1.4.0-rc2", -1},
		// A prerelease of a later version still outranks the earlier release.
		{"v1.3.1", "v1.4.0-rc1", -1},
		// Build metadata is ignored the same way a prerelease suffix is parsed off.
		{"v1.3.1+deadbeef", "v1.3.1+cafe", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// A dev build must never compare as newer than a real release: that is what stops
// `revert update` from silently downgrading a development checkout.
func TestCompareVersionsDevSortsBeforeAnyRelease(t *testing.T) {
	if got := compareVersions(DevVersion, "v0.0.1"); got != -1 {
		t.Errorf("compareVersions(%q, v0.0.1) = %d, want -1", DevVersion, got)
	}
}

func TestSplitVersion(t *testing.T) {
	cases := []struct {
		in  string
		num [3]int
		pre string
	}{
		{"v1.2.3", [3]int{1, 2, 3}, ""},
		{" v1.2.3 ", [3]int{1, 2, 3}, ""},
		{"1.2", [3]int{1, 2, 0}, ""},
		{"v2.0.0-rc1", [3]int{2, 0, 0}, "rc1"},
		{"v1.2.3+meta", [3]int{1, 2, 3}, ""},        // build metadata is dropped
		{"v1.2.3-rc1+meta", [3]int{1, 2, 3}, "rc1"}, // ...but the prerelease survives
		{"dev", [3]int{0, 0, 0}, ""},
		{"", [3]int{0, 0, 0}, ""},
	}
	for _, c := range cases {
		num, pre := splitVersion(c.in)
		if num != c.num || pre != c.pre {
			t.Errorf("splitVersion(%q) = %v,%q; want %v,%q", c.in, num, pre, c.num, c.pre)
		}
	}
}

func TestIsDevBuild(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	for _, v := range []string{"dev", ""} {
		Version = v
		if !IsDevBuild() {
			t.Errorf("IsDevBuild() = false for Version=%q, want true", v)
		}
	}
	Version = "v1.3.1"
	if IsDevBuild() {
		t.Error("IsDevBuild() = true for a stamped build, want false")
	}
}
