package api

import "testing"

func TestSanitizeOAuthLogin(t *testing.T) {
	cases := []struct {
		login, externalID, want string
	}{
		{"alice@example.com", "123", "alice"},
		{"bob@gmail.com", "g-1", "bob-g-1"},
		{"", "u-123", "gu-123"},
		{"Weird Name!", "x", "weird-name"},
	}
	for _, c := range cases {
		got := sanitizeOAuthLogin(c.login, c.externalID)
		if got != c.want {
			t.Errorf("sanitizeOAuthLogin(%q, %q) = %q, want %q", c.login, c.externalID, got, c.want)
		}
		if !usernameRe.MatchString(got) {
			t.Errorf("sanitizeOAuthLogin(%q, %q) = %q is not a valid username", c.login, c.externalID, got)
		}
	}
}
