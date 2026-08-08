package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSchedulerContextOperationsCancelBlockedRequests(t *testing.T) {
	tests := []struct {
		name    string
		call    func(context.Context, *PlatformService, Session) error
		session Session
	}{
		{
			name:    "session refresh",
			session: Session{Platform: PlatformSub2API, RefreshToken: "refresh-token"},
			call: func(ctx context.Context, service *PlatformService, session Session) error {
				_, err := service.RefreshSessionContext(ctx, session)
				return err
			},
		},
		{
			name:    "admin verification",
			session: Session{Platform: PlatformNewAPI, Cookie: "session=abc", UserID: "1"},
			call: func(ctx context.Context, service *PlatformService, session Session) error {
				return service.VerifyAdminContext(ctx, session)
			},
		},
		{
			name:    "new-api token list",
			session: Session{Platform: PlatformNewAPI, Cookie: "session=abc", UserID: "1"},
			call: func(ctx context.Context, service *PlatformService, session Session) error {
				_, err := service.ListNewAPITokensContext(ctx, session)
				return err
			},
		},
		{
			name:    "sub2api key list",
			session: Session{Platform: PlatformSub2API, AccessToken: "token", TokenType: "Bearer"},
			call: func(ctx context.Context, service *PlatformService, session Session) error {
				_, err := service.ListSub2APIKeysContext(ctx, session)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)

			service := NewPlatformService(NewHTTPClient(server.Client()))
			session := test.session
			session.BaseURL = server.URL
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- test.call(ctx, service, session) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("blocked request did not start")
			}
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("canceled request must return an error")
				}
			case <-time.After(time.Second):
				t.Fatal("request did not stop after context cancellation")
			}
		})
	}
}
