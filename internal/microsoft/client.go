package microsoft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	loginURL    = "https://login.microsoftonline.com"
	graphURL    = "https://graph.microsoft.com"
	redirectURL = "http://localhost/e5sub"
	scope       = "openid offline_access mail.read user.read"
)

type Client struct {
	http *http.Client
}

type User struct {
	ID                string `json:"id"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
}

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func NewClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{http: client}
}

func RegistrationURL() string {
	return "https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade"
}

func AuthorizationURL(clientID string) string {
	query := url.Values{
		"client_id":     {clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURL},
		"response_mode": {"query"},
		"scope":         {scope},
	}
	return loginURL + "/common/oauth2/v2.0/authorize?" + query.Encode()
}

func (c *Client) ExchangeCode(ctx context.Context, id, secret, code string) (string, error) {
	token, err := c.token(ctx, url.Values{
		"client_id":     {id},
		"client_secret": {secret},
		"grant_type":    {"authorization_code"},
		"scope":         {scope},
		"code":          {code},
		"redirect_uri":  {redirectURL},
	})
	if err != nil {
		return "", err
	}
	if token.RefreshToken == "" {
		return "", fmt.Errorf("authorization response has no refresh token")
	}
	return token.RefreshToken, nil
}

func (c *Client) Refresh(ctx context.Context, id, secret, refresh string) (string, string, error) {
	token, err := c.token(ctx, url.Values{
		"client_id":     {id},
		"client_secret": {secret},
		"grant_type":    {"refresh_token"},
		"scope":         {scope},
		"refresh_token": {refresh},
		"redirect_uri":  {redirectURL},
	})
	if err != nil {
		return "", "", err
	}
	if token.AccessToken == "" {
		return "", "", fmt.Errorf("refresh response has no access token")
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refresh
	}
	return token.RefreshToken, token.AccessToken, nil
}

func (c *Client) GetUserInfo(ctx context.Context, id, secret, refresh string) (string, User, error) {
	next, access, err := c.Refresh(ctx, id, secret, refresh)
	if err != nil {
		return "", User{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, graphURL+"/v1.0/me", nil)
	if err != nil {
		return "", User{}, err
	}
	request.Header.Set("Authorization", "Bearer "+access)
	response, err := c.do(ctx, request)
	if err != nil {
		return "", User{}, fmt.Errorf("request user info: %w", err)
	}
	defer response.Body.Close()
	if err := responseError(response); err != nil {
		return "", User{}, err
	}
	var user User
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&user); err != nil {
		return "", User{}, fmt.Errorf("decode user info: %w", err)
	}
	if user.ID == "" {
		return "", User{}, fmt.Errorf("user info has no ID")
	}
	return next, user, nil
}

func (c *Client) GetOutlookMails(ctx context.Context, id, secret, refresh string) (string, error) {
	next, access, err := c.Refresh(ctx, id, secret, refresh)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, graphURL+"/v1.0/me/messages?$top=1&$select=id", nil)
	if err != nil {
		return next, err
	}
	request.Header.Set("Authorization", "Bearer "+access)
	response, err := c.do(ctx, request)
	if err != nil {
		return next, fmt.Errorf("request Outlook messages: %w", err)
	}
	defer response.Body.Close()
	if err := responseError(response); err != nil {
		return next, err
	}
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return next, fmt.Errorf("read Outlook response: %w", err)
	}
	return next, nil
}

func (c *Client) token(ctx context.Context, values url.Values) (tokenResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL+"/common/oauth2/v2.0/token", strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.do(ctx, request)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("request Microsoft token: %w", err)
	}
	defer response.Body.Close()
	if err := responseError(response); err != nil {
		return tokenResponse{}, err
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return tokenResponse{}, fmt.Errorf("decode Microsoft token: %w", err)
	}
	if !strings.EqualFold(token.TokenType, "Bearer") {
		return tokenResponse{}, fmt.Errorf("unexpected Microsoft token type")
	}
	return token, nil
}

func responseError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&body)
	var code string
	if json.Unmarshal(body.Error, &code) != nil {
		var graph struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body.Error, &graph)
		code = graph.Code
	}
	// Keep response descriptions and unknown codes out of logs and Telegram messages.
	if code != "invalid_grant" && code != "invalid_client" && code != "unauthorized_client" {
		code = ""
	}
	return &APIError{Status: response.StatusCode, Code: code}
}

type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Microsoft API returned HTTP %d (%s)", e.Status, e.Code)
	}
	return fmt.Sprintf("Microsoft API returned HTTP %d", e.Status)
}

func RequiresAuthorization(err error) bool {
	var api *APIError
	return errors.As(err, &api) && (api.Status == 400 || api.Status == 401) && (api.Code == "invalid_grant" || api.Code == "invalid_client" || api.Code == "unauthorized_client")
}

// Retry safe transient responses with a bounded budget. Authorization-code exchanges
// are single use, so only GET requests and refresh-token exchanges are retried.
func (c *Client) do(ctx context.Context, request *http.Request) (*http.Response, error) {
	safe := request.Method == http.MethodGet
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			return nil, err
		}
		values, _ := url.ParseQuery(string(data))
		safe = values.Get("grant_type") == "refresh_token"
	}
	for attempt := 0; ; attempt++ {
		response, err := c.http.Do(request)
		if err != nil {
			return nil, err
		}
		if !safe || attempt >= 2 || (response.StatusCode != 429 && response.StatusCode < 500) {
			return response, nil
		}
		delay := time.Second << attempt
		if raw := response.Header.Get("Retry-After"); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				if seconds > 30 {
					return response, nil
				}
				delay = time.Duration(seconds) * time.Second
			} else if when, err := http.ParseTime(raw); err == nil {
				delay = time.Until(when)
				if delay < 0 {
					delay = 0
				}
			}
		}
		// Do not retry earlier than requested when the server asks for a long wait.
		if delay > 30*time.Second {
			return response, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		response.Body.Close()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		request = request.Clone(ctx)
		if request.GetBody != nil {
			request.Body, err = request.GetBody()
			if err != nil {
				return nil, err
			}
		}
	}
}
