package microsoft

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestGetUserInfoRotatesTokenAndUsesBearerHeader(t *testing.T) {
	var tokenRequests int
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/common/oauth2/v2.0/token":
			tokenRequests++
			if err := request.ParseForm(); err != nil || request.Form.Get("refresh_token") != "old-token" || request.Form.Get("client_secret") != "secret" {
				t.Fatalf("invalid refresh request: form=%v err=%v", request.Form, err)
			}
			return response(200, `{"token_type":"Bearer","access_token":"access","refresh_token":"new-token"}`), nil
		case "/v1.0/me":
			if request.Header.Get("Authorization") != "Bearer access" {
				t.Fatalf("missing bearer token: %q", request.Header.Get("Authorization"))
			}
			return response(200, `{"id":"user-1","userPrincipalName":"a@example.com","displayName":"A"}`), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})})
	next, user, err := client.GetUserInfo(context.Background(), "app", "secret", "old-token")
	if err != nil || next != "new-token" || user.ID != "user-1" || tokenRequests != 1 {
		t.Fatalf("token/user response incorrect: next=%q user=%+v requests=%d err=%v", next, user, tokenRequests, err)
	}
}

func TestGraphErrorDoesNotExposeResponseBody(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "login.microsoftonline.com" {
			return response(200, `{"token_type":"Bearer","access_token":"access","refresh_token":"new-token"}`), nil
		}
		return response(403, `{"error":"private-token-value"}`), nil
	})})
	next, err := client.GetOutlookMails(context.Background(), "app", "secret", "old-token")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "private-token-value") {
		t.Fatalf("unexpected Graph error: %v", err)
	}
	if next != "new-token" {
		t.Fatalf("rotated token lost: %q", next)
	}
}

func TestGraphRetriesThrottlingAndLimitsPayload(t *testing.T) {
	var calls int
	client := NewClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "login.microsoftonline.com" {
			return response(200, `{"token_type":"Bearer","access_token":"access","refresh_token":"new"}`), nil
		}
		if request.URL.Query().Get("$top") != "1" || request.URL.Query().Get("$select") != "id" {
			t.Fatal("unbounded mail query")
		}
		calls++
		if calls < 3 {
			r := response(429, `{}`)
			r.Header.Set("Retry-After", "0")
			return r, nil
		}
		return response(200, `{"value":[]}`), nil
	})})
	next, err := client.GetOutlookMails(context.Background(), "id", "secret", "old")
	if err != nil || calls != 3 || next != "new" {
		t.Fatalf("retry failed: calls=%d token=%q err=%v", calls, next, err)
	}
}

func TestRetryCancellationAndLongDelay(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "long-delay", true: "cancelled"}[cancel], func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			calls := 0
			client := NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				r := response(429, `{}`)
				if cancel {
					stop()
					r.Header.Set("Retry-After", "1")
				} else {
					r.Header.Set("Retry-After", "60")
				}
				return r, nil
			})})
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, graphURL, nil)
			result, err := client.do(ctx, request)
			if result != nil {
				result.Body.Close()
			}
			if calls != 1 || (cancel && !errors.Is(err, context.Canceled)) || (!cancel && (err != nil || result.StatusCode != 429)) {
				t.Fatalf("bad retry budget: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestAuthorizationErrorsAreClassifiedWithoutSecrets(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_client", "unauthorized_client", "private-secret"} {
		err := responseError(response(400, `{"error":"`+code+`","error_description":"private-secret"}`))
		if RequiresAuthorization(err) != (code != "private-secret") || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("unsafe error classification: %v", err)
		}
	}
}

func TestTokenRetryReplaysRefreshBodyButNeverAuthorizationCode(t *testing.T) {
	for _, grant := range []string{"refresh_token", "authorization_code"} {
		t.Run(grant, func(t *testing.T) {
			calls := 0
			client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != grant || r.Form.Get("client_secret") != "secret" {
					t.Fatal("retry lost token request body")
				}
				result := response(503, `{}`)
				result.Header.Set("Retry-After", "0")
				return result, nil
			})})
			if grant == "refresh_token" {
				_, _, _ = client.Refresh(context.Background(), "app", "secret", "refresh")
			} else {
				_, _ = client.ExchangeCode(context.Background(), "app", "secret", "code")
			}
			want := 1
			if grant == "refresh_token" {
				want = 3
			}
			if calls != want {
				t.Fatalf("incorrect retry limit: got=%d want=%d", calls, want)
			}
		})
	}
}
