package web

import "testing"

func TestResourcePresentation(t *testing.T) {
	for _, tc := range []struct {
		size int64
		want string
	}{{0, "0 B"}, {1024, "1.0 KiB"}, {1199672, "1.1 MiB"}} {
		if got := resourceSize(tc.size); got != tc.want {
			t.Errorf("size %d: got %q, want %q", tc.size, got, tc.want)
		}
	}
	for _, tc := range []struct{ input, want string }{{"CC BY 4.0 — https://creativecommons.org/licenses/by/4.0/", "CC BY 4.0"}, {"署名许可", "署名许可"}, {"", ""}, {"https://example.com/license", "https://example.com/license"}} {
		if got := resourceLicenseSummary(tc.input); got != tc.want {
			t.Errorf("license %q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}
