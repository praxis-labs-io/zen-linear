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
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	if tag == "" {
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
