package update

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want version
		ok   bool
	}{
		{name: "a plain tag", tag: "v0.3.0", want: version{nums: [3]int{0, 3, 0}}, ok: true},
		{name: "without the v", tag: "0.3.0", want: version{nums: [3]int{0, 3, 0}}, ok: true},
		{name: "surrounded by space", tag: "  v1.2.3  ", want: version{nums: [3]int{1, 2, 3}}, ok: true},
		{name: "a pre-release", tag: "v0.2.1-rc.1", want: version{nums: [3]int{0, 2, 1}, pre: true}, ok: true},
		{name: "build metadata carrying a dash", tag: "v1.0.0+build-7", want: version{nums: [3]int{1, 0, 0}}, ok: true},
		{name: "double digits", tag: "v0.10.2", want: version{nums: [3]int{0, 10, 2}}, ok: true},
		{name: "the unstamped build", tag: "dev", ok: false},
		{name: "two fields", tag: "v0.3", ok: false},
		{name: "four fields", tag: "v0.3.0.1", ok: false},
		{name: "not a number", tag: "v0.x.0", ok: false},
		{name: "negative", tag: "v0.-1.0", ok: false},
		{name: "empty", tag: "", ok: false},
		{name: "the v alone", tag: "v", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseVersion(tt.tag)
			if ok != tt.ok {
				t.Fatalf("parseVersion(%q) ok = %v, want %v", tt.tag, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("parseVersion(%q) = %+v, want %+v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "a newer minor", current: "0.2.0", latest: "v0.3.0", want: true},
		{name: "a newer patch", current: "0.3.0", latest: "v0.3.1", want: true},
		{name: "a newer major", current: "0.9.9", latest: "v1.0.0", want: true},
		{name: "the same version", current: "0.3.0", latest: "v0.3.0"},
		{name: "an older release", current: "0.3.0", latest: "v0.2.0"},
		// The reason the comparison is numeric rather than a string compare.
		{name: "ten is ahead of nine", current: "0.9.0", latest: "v0.10.0", want: true},
		{name: "nine is not ahead of ten", current: "0.10.0", latest: "v0.9.0"},
		{name: "a patch past nine", current: "0.1.9", latest: "v0.1.10", want: true},
		// Somebody who installed an rc is behind the final it led to.
		{name: "the final over its rc", current: "0.3.0-rc.1", latest: "v0.3.0", want: true},
		{name: "an rc is not ahead of its final", current: "0.3.0", latest: "v0.3.0-rc.1"},
		{name: "a later minor over an rc", current: "0.3.0-rc.1", latest: "v0.4.0", want: true},
		// Nothing ranks against a tag that will not parse, in either direction.
		{name: "an unstamped build", current: "dev", latest: "v0.3.0"},
		{name: "an unparseable tag", current: "0.3.0", latest: "nightly"},
		{name: "neither parses", current: "dev", latest: "nightly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
