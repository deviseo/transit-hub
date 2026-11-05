package client_config

import "testing"

func TestSameOwnerMatchesTransitLink(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://api.example.com", "https://hub.example.com/transithub", true},
		{"https://example.com", "https://hub.example.com", true},
		{"https://API.example.com", "https://hub.Example.com", true},
		{"https://api.example.com.cn", "https://hub.example.com.cn", true},
		{"http://1.2.3.4:3000", "http://1.2.3.4:8080", true},
		{"http://localhost:3000", "http://localhost:5555", true},
		{"https://api.example.com", "https://hub.other.com", false},
		{"https://a.example.com.cn", "https://b.other.com.cn", false},
		{"https://relay.github.io", "https://evil.github.io", false},
		{"http://1.2.3.4", "http://1.2.3.5", false},
		{"http://localhost:3000", "http://127.0.0.1:3000", false},
		{"https://api.example.com", "", false},
	}
	for _, c := range cases {
		if got := sameOwner(c.a, c.b); got != c.want {
			t.Errorf("sameOwner(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
