package microsoft

import (
	"context"
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
	_, err := client.GetOutlookMails(context.Background(), "app", "secret", "old-token")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "private-token-value") {
		t.Fatalf("unexpected Graph error: %v", err)
	}
}
