package tickets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyAgainstSub2API(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer good-token":
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":42,"email":"alice@example.com","username":"alice","role":"user"}}`))
		case "Bearer business-error":
			_, _ = w.Write([]byte(`{"code":401,"message":"token expired"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	verifier := NewSiteVerifier(nil)
	ctx := context.Background()

	user, err := verifier.Verify(ctx, "sub2api", server.URL+"/", Credential{Kind: "bearer", Token: "good-token"})
	if err != nil || user != (SiteUser{Platform: "sub2api", ID: "42", Username: "alice", Email: "alice@example.com"}) {
		t.Fatalf("verify: %+v %v", user, err)
	}
	for _, token := range []string{"expired", "business-error"} {
		if _, err := verifier.Verify(ctx, "sub2api", server.URL, Credential{Kind: "bearer", Token: token}); errorKey(err) != ErrorSiteLoginExpired {
			t.Fatalf("%s: want login expired, got %v", token, err)
		}
	}
	// sub2api 没有 cookie 登录方式。
	if _, err := verifier.Verify(ctx, "sub2api", server.URL, Credential{Kind: "cookie", Cookie: "session=x", UserID: "42"}); errorKey(err) != ErrorInvalid {
		t.Fatalf("cookie on sub2api: %v", err)
	}
}

func TestVerifyAgainstNewAPI(t *testing.T) {
	var gotCookie, gotUser string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		gotCookie, gotUser = r.Header.Get("Cookie"), r.Header.Get("New-Api-User")
		switch {
		case r.Header.Get("Authorization") == "Bearer nap_good":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"bob","display_name":"Bob","email":""}}`))
		case gotCookie == "session=legacy" && gotUser == "7":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"username":"bob"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"message":"未登录"}`))
		}
	}))
	defer server.Close()
	verifier := NewSiteVerifier(nil)
	ctx := context.Background()

	user, err := verifier.Verify(ctx, "newapi", server.URL, Credential{Kind: "bearer", Token: "nap_good"})
	if err != nil || user.ID != "7" || user.Username != "Bob" || user.Platform != "newapi" {
		t.Fatalf("token: %+v %v", user, err)
	}
	user, err = verifier.Verify(ctx, "newapi", server.URL, Credential{Kind: "cookie", Cookie: "session=legacy", UserID: "7"})
	if err != nil || user.ID != "7" || user.Username != "bob" || gotUser != "7" {
		t.Fatalf("legacy session: %+v %v", user, err)
	}
	if _, err := verifier.Verify(ctx, "newapi", server.URL, Credential{Kind: "bearer", Token: "nap_bad"}); errorKey(err) != ErrorSiteLoginExpired {
		t.Fatalf("rejected token: %v", err)
	}
	// 声称的用户 ID 和站点返回的不一致：不认。
	if _, err := verifier.Verify(ctx, "newapi", server.URL, Credential{Kind: "cookie", Cookie: "session=legacy", UserID: "8"}); errorKey(err) == "" {
		t.Fatalf("a mismatched user id must be rejected")
	}
}

func TestVerifyRejectsBadCredentialsBeforeCallingTheSite(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	verifier := NewSiteVerifier(nil)
	cases := map[string]Credential{
		"empty token":      {Kind: "bearer"},
		"header injection": {Kind: "bearer", Token: "a\r\nX-Evil: 1"},
		"unknown kind":     {Kind: "password", Token: "x"},
		"bad user id":      {Kind: "cookie", Cookie: "session=x", UserID: "7; drop"},
	}
	for name, credential := range cases {
		if _, err := verifier.Verify(context.Background(), "newapi", server.URL, credential); errorKey(err) != ErrorInvalid {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
	if called {
		t.Fatal("the site must not be called with an invalid credential")
	}
}

func TestVerifyReportsAnUnreachableSite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	verifier := NewSiteVerifier(nil)
	if _, err := verifier.Verify(context.Background(), "sub2api", server.URL, Credential{Kind: "bearer", Token: "x"}); errorKey(err) != ErrorSiteUnavailable {
		t.Fatalf("5xx: %v", err)
	}
	server.Close()
	if _, err := verifier.Verify(context.Background(), "sub2api", server.URL, Credential{Kind: "bearer", Token: "x"}); errorKey(err) != ErrorSiteUnavailable {
		t.Fatalf("connection refused: %v", err)
	}
}
