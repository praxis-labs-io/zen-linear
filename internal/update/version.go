package update

import (
	"strconv"
	"strings"
)

type version struct {
	nums [3]int
	pre  bool
}

func parseVersion(tag string) (version, bool) {
	tag = strings.TrimPrefix(tag, "v")
	if tag == "" || strings.IndexFunc(tag, outsideSemver) >= 0 {
		return version{}, false
	}

	var parsed version
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

	return running.pre && !published.pre
}

// A tag is printed to the terminal, so anything semver does not allow could carry an escape sequence.
func outsideSemver(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return false
	case r == '.', r == '-', r == '+':
		return false
	}
	return true
}
