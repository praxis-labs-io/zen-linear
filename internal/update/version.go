package update

import (
	"strconv"
	"strings"
)

// version is a release's three numbers plus whether a pre-release suffix
// followed them. The suffix is not ordered beyond its presence: Linear's
// releases only ever carry -rc.N, and ranking two candidates against each
// other would be answering a question nothing here asks.
type version struct {
	nums [3]int
	pre  bool
}

// parseVersion reads a release tag. It takes "v0.3.0", "0.3.0" and
// "0.3.0-rc.1", and reports false for anything it cannot make three numbers
// out of, which is what keeps an unrecognized tag from being ranked.
func parseVersion(tag string) (version, bool) {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	if tag == "" {
		return version{}, false
	}

	var parsed version
	// Build metadata is dropped before the pre-release split, since a "+" may
	// carry a "-" of its own.
	if plus := strings.IndexByte(tag, '+'); plus >= 0 {
		tag = tag[:plus]
	}
	if dash := strings.IndexByte(tag, '-'); dash >= 0 {
		parsed.pre = true
		tag = tag[:dash]
	}

	fields := strings.Split(tag, ".")
	if len(fields) != 3 {
		return version{}, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return version{}, false
		}
		parsed.nums[i] = n
	}

	return parsed, true
}

// isNewer reports whether latest is a release the current build is behind.
// Both tags have to parse: an unrecognized one on either side is no answer
// rather than a nudge nobody can act on.
func isNewer(current, latest string) bool {
	running, ok := parseVersion(current)
	if !ok {
		return false
	}
	published, ok := parseVersion(latest)
	if !ok {
		return false
	}

	for i := range running.nums {
		if published.nums[i] != running.nums[i] {
			return published.nums[i] > running.nums[i]
		}
	}

	// Same three numbers, so the only release ahead is the final one over the
	// pre-release that led to it. Somebody running v0.3.0-rc.1 is behind
	// v0.3.0, and that is the whole of what the suffix is read for.
	return running.pre && !published.pre
}
