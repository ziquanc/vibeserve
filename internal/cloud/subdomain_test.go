package cloud

import "testing"

func TestSubdomain(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		override string
		want     string
	}{
		{"basic", "Coffee Corner", "", "coffee-corner"},
		{"already kebab", "my-shop", "", "my-shop"},
		{"special chars", "Café & Co!", "", "caf-co"},
		{"with numbers", "Shop 24", "", "shop-24"},
		{"unicode collapsed", "東京 cafe", "", "cafe"},
		{"empty fallback", "", "", "project"},
		{"leading/trailing dashes trimmed", "  --hello--  ", "", "hello"},
		{"long truncated", "a-very-long-name-that-just-goes-on-and-on-and-on-and-on-and-on-and-on", "", "a-very-long-name-that-just-goes-on-and-on-and-on-a"},
		{"override wins", "Coffee Corner", "my-coffee", "my-coffee"},
		{"override sanitized", "ignored", "BAD NAME!", "bad-name"},
		{"override empty falls back to derived", "Coffee Corner", "", "coffee-corner"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Subdomain(tc.input, tc.override)
			if got != tc.want {
				t.Errorf("Subdomain(%q, %q) = %q, want %q", tc.input, tc.override, got, tc.want)
			}
		})
	}
}
