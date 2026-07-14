package lottery

import "testing"

func TestNormalizeSrcHostRejectsPrivateTargetsByDefault(t *testing.T) {
	for _, value := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://192.168.1.10:8080",
		"http://[::1]:8080",
	} {
		if _, err := normalizeSrcHost(value); err == nil {
			t.Fatalf("expected %q to be rejected in strict mode", value)
		}
	}
}

func TestNormalizeSrcHostAllowsPrivateTargetsForLocalDebugging(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080/path?q=1":  "http://localhost:8080",
		"http://127.0.0.1:8080/path":      "http://127.0.0.1:8080",
		"http://192.168.1.10:8080/path":   "http://192.168.1.10:8080",
		"http://[::1]:8080/path#fragment": "http://[::1]:8080",
	}
	for value, want := range cases {
		got, err := normalizeSrcHostWithPrivateTargets(value, true)
		if err != nil {
			t.Fatalf("expected %q to be allowed, got err=%v", value, err)
		}
		if got != want {
			t.Fatalf("normalize %q = %q, want %q", value, got, want)
		}
	}
}

func TestNormalizeSrcHostStillRejectsSpecialTargetsWhenLocalDebuggingEnabled(t *testing.T) {
	for _, value := range []string{
		"http://0.0.0.0:8080",
		"http://169.254.169.254",
		"http://[fe80::1]:8080",
		"http://224.0.0.1:8080",
	} {
		if _, err := normalizeSrcHostWithPrivateTargets(value, true); err == nil {
			t.Fatalf("expected special target %q to remain rejected", value)
		}
	}
}
