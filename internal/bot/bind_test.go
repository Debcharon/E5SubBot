package bot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
)

type testSettings struct{ cfg config.Config }

func (s testSettings) Current() config.Config { return s.cfg }

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestExpiredBindingsDropCredentials(t *testing.T) {
	now := time.Now()
	b := &Bot{bindings: map[int64]binding{
		1: {id: "old", secret: "secret", expires: now},
		2: {id: "active", secret: "secret", expires: now.Add(time.Minute)},
	}}
	b.expireBindings(now)
	if len(b.bindings) != 1 || b.bindings[2].id != "active" {
		t.Fatalf("incorrect session cleanup: %v", b.bindings)
	}
}

func TestConcurrentBindingCreatesOneAccountAndCancelClearsSession(t *testing.T) {
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected Telegram request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"chat":{"id":101,"type":"private"}}}`)
	}))
	defer telegram.Close()
	api, err := tb.NewBot(tb.Settings{Offline: true, Token: "test", URL: telegram.URL, Client: telegram.Client()})
	if err != nil {
		t.Fatal(err)
	}
	store, err := account.Open(config.Config{Database: "sqlite", Table: "users", SQLitePath: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var exchanges atomic.Int32
	ms := microsoft.NewClient(&http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"user-1","displayName":"test","userPrincipalName":"test@example.com"}`
		if strings.HasSuffix(r.URL.Path, "/token") {
			_ = r.ParseForm()
			if r.Form.Get("grant_type") == "authorization_code" {
				exchanges.Add(1)
			}
			body = `{"token_type":"Bearer","access_token":"access","refresh_token":"refresh"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	b := &Bot{api: api, ctx: context.Background(), logger: zap.NewNop(), accounts: store, microsoft: ms,
		settings: testSettings{config.Config{BindMax: 1}}, bindings: map[int64]binding{
			101: {step: waitingForAuthorization, id: "app", secret: "secret", expires: time.Now().Add(time.Minute)},
		}}
	message := &tb.Message{Chat: &tb.Chat{ID: 101}, ReplyTo: &tb.Message{}, Text: "http://localhost/e5sub?code=test alias"}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.onText(message) }()
	}
	wg.Wait()
	clients, err := store.ListByUser(b.ctx, 101)
	if err != nil || len(clients) != 1 || exchanges.Load() != 1 || len(b.bindings) != 0 {
		t.Fatalf("duplicate binding: count=%d exchanges=%d sessions=%d err=%v", len(clients), exchanges.Load(), len(b.bindings), err)
	}
	b.onBind(message)
	b.onCancel(message)
	if len(b.bindings) != 0 {
		t.Fatal("cancel retained binding credentials")
	}
}
