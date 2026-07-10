package core

import (
	"strconv"
	"strings"
)

// DevVersion is what an unstamped build reports.
const DevVersion = "dev"

// Version is the release tag this binary was built from. The Windows packer stamps it at
// link time:
//
//	go build -ldflags "-X github.com/violetvandal/revert/internal/core.Version=v1.3.1"
//
// A build without that stamp reports DevVersion and refuses to self-update — it has no
// way to tell whether a published release is newer or older than what it is running, and
// a wrong guess would silently downgrade a development checkout.
var Version = DevVersion

// IsDevBuild reports whether this binary carries no release stamp.
func IsDevBuild() bool { return Version == "" || Version == DevVersion }

// compareVersions orders two release tags ("v1.3.1", "1.4", "v2.0.0-rc1"), returning -1
// if a sorts before b, 0 if they're equal, +1 if a sorts after b.
//
// Missing components read as zero (v1.3 == v1.3.0). A prerelease suffix sorts *before*
// its plain release (v1.4.0-rc1 < v1.4.0), per semver precedence — so an rc never looks
// newer than the release it precedes.
func compareVersions(a, b string) int {
	an, apre := splitVersion(a)
	bn, bpre := splitVersion(b)
	for i := range an {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case apre == "" && bpre == "":
		return 0
	case apre == "": // a is the plain release, b a prerelease of it
		return 1
	case bpre == "":
		return -1
	}
	return strings.Compare(apre, bpre)
}

// splitVersion parses "vX.Y.Z-pre+build" into its numeric triple and prerelease suffix.
// Build metadata is discarded: semver excludes it from precedence, so two tags differing
// only after "+" are the same release.
//
// Unparsable components read as zero rather than erroring: version strings reach us from
// GitHub tags, and an odd one should degrade to "not newer", never crash an update.
func splitVersion(v string) ([3]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i] // build metadata: ignored for precedence
	}
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out[i] = n
	}
	return out, pre
}
