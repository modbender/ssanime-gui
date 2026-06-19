package extension

import "testing"

func TestShouldUpdateExtension(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		index     string
		want      bool
	}{
		{"equal semver no-op", "1.2.3", "1.2.3", false},
		{"index newer semver", "1.2.3", "1.3.0", true},
		{"index older semver no downgrade", "1.3.0", "1.2.3", false},
		{"index newer patch", "1.0.0", "1.0.1", true},
		{"empty index version never updates", "1.0.0", "", false},
		{"empty installed treated as update", "", "1.0.0", true},
		{"both empty no-op", "", "", false},
		{"non-semver inequality updates", "rev-a", "rev-b", true},
		{"non-semver equal no-op", "rev-a", "rev-a", false},
		{"semver-vs-garbage inequality updates", "1.0.0", "latest", true},
		{"v-prefixed equal", "v1.2.3", "1.2.3", false},
		{"v-prefixed newer", "v1.2.3", "v1.3.0", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldUpdateExtension(c.installed, c.index); got != c.want {
				t.Errorf("shouldUpdateExtension(%q, %q) = %v, want %v", c.installed, c.index, got, c.want)
			}
		})
	}
}
