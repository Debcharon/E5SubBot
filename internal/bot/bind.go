package bot

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
)

type bindingStep uint8

const (
	waitingForCredentials bindingStep = iota + 1
	waitingForAuthorization
)

type binding struct {
	step    bindingStep
	id      string
	secret  string
	expires time.Time
}

const bindingTTL = 15 * time.Minute

func (b *Bot) expireBindings(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, state := range b.bindings {
		if !now.Before(state.expires) {
			delete(b.bindings, id)
		}
	}
}

func (b *Bot) onCancel(message *tb.Message) {
	lock := &b.bindLocks[uint64(message.Chat.ID)%uint64(len(b.bindLocks))]
	lock.Lock()
	defer lock.Unlock()
	b.mu.Lock()
	delete(b.bindings, message.Chat.ID)
	b.mu.Unlock()
	b.send(message.Chat, "Binding cancelled.")
}

func (b *Bot) onBind(message *tb.Message) {
	lock := &b.bindLocks[uint64(message.Chat.ID)%uint64(len(b.bindLocks))]
	lock.Lock()
	defer lock.Unlock()
	b.mu.Lock()
	b.bindings[message.Chat.ID] = binding{step: waitingForCredentials, expires: time.Now().Add(bindingTTL)}
	b.mu.Unlock()
	b.send(message.Chat, fmt.Sprintf("Register application [Directly](%s)", microsoft.RegistrationURL()), tb.ModeMarkdown)
	b.send(message.Chat, "Please reply with your `client_id` + `client_secret`", &tb.SendOptions{
		ParseMode: tb.ModeMarkdown, ReplyMarkup: &tb.ReplyMarkup{ForceReply: true},
	})
}

func (b *Bot) onText(message *tb.Message) {
	lock := &b.bindLocks[uint64(message.Chat.ID)%uint64(len(b.bindLocks))]
	lock.Lock()
	defer lock.Unlock()
	b.mu.Lock()
	state, ok := b.bindings[message.Chat.ID]
	if ok && !time.Now().Before(state.expires) {
		delete(b.bindings, message.Chat.ID)
		ok = false
	}
	b.mu.Unlock()
	if !ok {
		b.send(message.Chat, "No active binding session. Send /bind to start or /help for help.")
		return
	}
	switch state.step {
	case waitingForCredentials:
		b.onCredentials(message)
	case waitingForAuthorization:
		b.onAuthorization(message, state)
	}
}

func (b *Bot) onCredentials(message *tb.Message) {
	if !message.IsReply() {
		b.send(message.Chat, "WARN: Please reply to the bot message to bind")
		return
	}
	parts := strings.Fields(message.Text)
	if len(parts) != 2 {
		b.send(message.Chat, "WARN: Expected client_id and client_secret separated by a space")
		return
	}
	id, secret := parts[0], parts[1]
	b.mu.Lock()
	b.bindings[message.Chat.ID] = binding{step: waitingForAuthorization, id: id, secret: secret, expires: time.Now().Add(bindingTTL)}
	b.mu.Unlock()
	b.send(message.Chat, fmt.Sprintf("Authorize account [Directly](%s)", microsoft.AuthorizationURL(id)), tb.ModeMarkdown)
	b.send(message.Chat, "Please reply with http://localhost/... and an alias", &tb.SendOptions{
		ReplyMarkup: &tb.ReplyMarkup{ForceReply: true},
	})
}

func (b *Bot) onAuthorization(message *tb.Message, state binding) {
	if !message.IsReply() {
		b.send(message.Chat, "WARN: Please reply to the bot message to bind")
		return
	}
	parts := strings.Fields(message.Text)
	if len(parts) != 2 {
		b.send(message.Chat, "WARN: Expected authorization URL and alias separated by a space")
		return
	}
	callbackURL, err := url.Parse(parts[0])
	if err != nil || callbackURL.Query().Get("code") == "" {
		b.send(message.Chat, "WARN: Authorization URL has no code")
		return
	}
	clients, err := b.accounts.ListByUser(b.ctx, message.Chat.ID)
	if err != nil {
		b.logger.Error("list accounts during bind", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not check existing accounts")
		return
	}
	if len(clients) >= b.settings.Current().BindMax {
		b.send(message.Chat, "WARN: You have reached the maximum number of accounts")
		return
	}
	exists, err := b.accounts.Exists(b.ctx, message.Chat.ID, state.id)
	if err != nil {
		b.logger.Error("check existing account", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not check existing accounts")
		return
	}
	if exists {
		b.send(message.Chat, "WARN: This application is already bound")
		return
	}
	b.send(message.Chat, "Binding now...")
	refresh, err := b.microsoft.ExchangeCode(b.ctx, state.id, state.secret, callbackURL.Query().Get("code"))
	if err != nil {
		b.logger.Warn("exchange Microsoft authorization code", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not get a refresh token")
		return
	}
	refresh, user, err := b.microsoft.GetUserInfo(b.ctx, state.id, state.secret, refresh)
	if err != nil {
		b.logger.Warn("get Microsoft user info", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not get Microsoft user info")
		return
	}
	hash := md5.Sum([]byte(user.ID))
	msID := fmt.Sprintf("%x", hash)[8:24]
	client := &account.Client{
		TelegramID: message.Chat.ID, RefreshToken: refresh, MicrosoftID: msID,
		Alias: parts[1], ClientID: state.id, ClientSecret: state.secret,
	}
	if err := b.accounts.Create(b.ctx, client); err != nil {
		b.logger.Error("save bound account", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not save account")
		return
	}
	b.mu.Lock()
	delete(b.bindings, message.Chat.ID)
	b.mu.Unlock()
	b.send(message.Chat, fmt.Sprintf("ms_id: %s\nuserPrincipalName: %s\ndisplayName: %s", msID, user.UserPrincipalName, user.DisplayName))
	b.send(message.Chat, "Bind account successfully!")
}
