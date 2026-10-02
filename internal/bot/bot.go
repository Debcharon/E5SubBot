package bot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/config"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
	"github.com/Debcharon/E5SubBot/internal/renewal"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	tb "gopkg.in/tucnak/telebot.v2"
)

type AccountStore interface {
	Create(context.Context, *account.Client) error
	DeleteForUser(context.Context, int, int64) (bool, error)
	Exists(context.Context, int64, string) (bool, error)
	GetForUser(context.Context, int, int64) (account.Client, error)
	ListByUser(context.Context, int64) ([]account.Client, error)
}

type Settings interface {
	Current() config.Config
}

type Bot struct {
	api       *tb.Bot
	settings  Settings
	accounts  AccountStore
	microsoft *microsoft.Client
	runner    *renewal.Runner
	logger    *zap.Logger
	ctx       context.Context
	mu        sync.Mutex
	bindings  map[int64]binding
	bindLocks [64]sync.Mutex
	taskMu    sync.Mutex
	taskWG    sync.WaitGroup
	stopping  bool
}

func New(ctx context.Context, settings Settings, accounts AccountStore, ms *microsoft.Client, runner *renewal.Runner, logger *zap.Logger) (*Bot, error) {
	cfg := settings.Current()
	poller := tb.NewMiddlewarePoller(&tb.LongPoller{Timeout: 15 * time.Second}, func(update *tb.Update) bool {
		return update.Message == nil || update.Message.Private()
	})
	botSettings := tb.Settings{Token: cfg.BotToken, Poller: poller, Client: &http.Client{Timeout: 30 * time.Second}}
	if cfg.Socks5 != "" {
		dialer, err := proxy.SOCKS5("tcp", cfg.Socks5, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("configure Telegram proxy: %w", err)
		}
		transport := &http.Transport{DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}}
		botSettings.Client = &http.Client{Timeout: 30 * time.Second, Transport: transport}
	}
	api, err := tb.NewBot(botSettings)
	if err != nil {
		return nil, fmt.Errorf("create Telegram bot: %w", err)
	}
	b := &Bot{
		api: api, settings: settings, accounts: accounts, microsoft: ms,
		runner: runner, logger: logger, ctx: ctx, bindings: make(map[int64]binding),
	}
	b.registerHandlers()
	return b, nil
}

func (b *Bot) registerHandlers() {
	b.api.Handle("/start", b.onStart)
	b.api.Handle("/help", b.onHelp)
	b.api.Handle("/my", b.onMy)
	b.api.Handle("/bind", b.onBind)
	b.api.Handle("/cancel", b.onCancel)
	b.api.Handle("/unbind", b.onUnbind)
	b.api.Handle("/export", b.onExport)
	b.api.Handle("/task", b.onTask)
	b.api.Handle("/status", b.onStatus)
	b.api.Handle("/log", b.onLog)
	b.api.Handle(tb.OnText, b.onText)
	b.api.Handle(&tb.InlineButton{Unique: "account-view"}, b.onView)
	b.api.Handle(&tb.InlineButton{Unique: "account-unbind"}, b.onUnbindCallback)
}

func (b *Bot) Start() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-b.ctx.Done():
				return
			case now := <-ticker.C:
				b.expireBindings(now)
			}
		}
	}()
	b.logger.Info("Telegram bot started", zap.Int64("bot_id", b.api.Me.ID), zap.String("username", b.api.Me.Username))
	b.api.Start()
}

func (b *Bot) Stop() {
	b.taskMu.Lock()
	b.stopping = true
	b.taskMu.Unlock()
	b.api.Stop()
	b.taskWG.Wait()
}

func (b *Bot) send(to tb.Recipient, message interface{}, options ...interface{}) {
	for attempt := 0; attempt < 3; attempt++ {
		if b.ctx.Err() != nil {
			return
		}
		_, err := b.api.Send(to, message, options...)
		if err == nil {
			return
		}
		var flood tb.FloodError
		if attempt == 2 || !errors.As(err, &flood) || flood.RetryAfter < 0 || flood.RetryAfter > 30 {
			b.logger.Warn("send Telegram message", zap.Error(err))
			return
		}
		timer := time.NewTimer(time.Duration(flood.RetryAfter) * time.Second)
		select {
		case <-b.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (b *Bot) respond(callback *tb.Callback) {
	if err := b.api.Respond(callback); err != nil {
		b.logger.Warn("respond to Telegram callback", zap.Error(err))
	}
}

func (b *Bot) isAdmin(userID int64) bool {
	for _, adminID := range b.settings.Current().Admins {
		if adminID == userID {
			return true
		}
	}
	return false
}
