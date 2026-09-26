package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
)

const welcome = "Welcome to use E5SubBot!"

const help = `Command:
/my Check your account info
/bind Bind new account
/unbind Unbind account
/export Export account info (JSON)
/help Help

Open source:
https://github.com/Debcharon/E5SubBot`

func (b *Bot) onStart(message *tb.Message) {
	b.send(message.Sender, welcome)
	b.onHelp(message)
}

func (b *Bot) onHelp(message *tb.Message) {
	b.send(message.Sender, help+"\n"+b.settings.Current().Notice)
}

func (b *Bot) onMy(message *tb.Message) {
	clients, err := b.accounts.ListByUser(b.ctx, message.Chat.ID)
	if err != nil {
		b.logger.Error("list accounts", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not load accounts")
		return
	}
	var rows [][]tb.InlineButton
	for _, client := range clients {
		rows = append(rows, []tb.InlineButton{{
			Unique: "account-view", Text: client.Alias, Data: strconv.Itoa(client.ID),
		}})
	}
	b.send(message.Chat, fmt.Sprintf("Choose an account\n\nBindings: %d/%d", len(clients), b.settings.Current().BindMax),
		&tb.ReplyMarkup{InlineKeyboard: rows})
}

func (b *Bot) onView(callback *tb.Callback) {
	defer b.respond(callback)
	id, err := strconv.Atoi(callback.Data)
	if err != nil || callback.Sender == nil {
		return
	}
	client, err := b.accounts.GetForUser(b.ctx, id, callback.Sender.ID)
	if err != nil {
		b.send(callback.Sender, "Account not found")
		return
	}
	b.send(callback.Sender, fmt.Sprintf("Detail\nAlias: %s\nms_id: %s\nclient_id: %s\nclient_secret: %s\nLast updated: %s",
		client.Alias, client.MicrosoftID, client.ClientID, client.ClientSecret,
		time.Unix(client.UpdatedAtUnix, 0).Format("2006-01-02 15:04:05")))
}

func (b *Bot) onUnbind(message *tb.Message) {
	clients, err := b.accounts.ListByUser(b.ctx, message.Chat.ID)
	if err != nil {
		b.logger.Error("list accounts for unbind", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not load accounts")
		return
	}
	var rows [][]tb.InlineButton
	for _, client := range clients {
		rows = append(rows, []tb.InlineButton{{
			Unique: "account-unbind", Text: client.Alias, Data: strconv.Itoa(client.ID),
		}})
	}
	b.send(message.Chat, fmt.Sprintf("Select an account to unbind\n\nBindings: %d/%d", len(clients), b.settings.Current().BindMax),
		&tb.ReplyMarkup{InlineKeyboard: rows})
}

func (b *Bot) onUnbindCallback(callback *tb.Callback) {
	defer b.respond(callback)
	id, err := strconv.Atoi(callback.Data)
	if err != nil || callback.Sender == nil {
		return
	}
	deleted, err := b.accounts.DeleteForUser(b.ctx, id, callback.Sender.ID)
	if err != nil {
		b.logger.Error("delete account", zap.Error(err))
		b.send(callback.Sender, "ERROR: Could not unbind account")
		return
	}
	if !deleted {
		b.send(callback.Sender, "Account not found")
		return
	}
	b.send(callback.Sender, "Unbind account successfully!")
}

func (b *Bot) onExport(message *tb.Message) {
	clients, err := b.accounts.ListByUser(b.ctx, message.Chat.ID)
	if err != nil {
		b.logger.Error("list accounts for export", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not export accounts")
		return
	}
	if len(clients) == 0 {
		b.send(message.Chat, "WARN: You have not bound an account yet")
		return
	}
	type entry struct {
		Alias        string `json:"alias"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
		Other        string `json:"other"`
	}
	export := make([]entry, 0, len(clients))
	for _, client := range clients {
		export = append(export, entry{
			Alias: client.Alias, ClientID: client.ClientID, ClientSecret: client.ClientSecret,
			RefreshToken: client.RefreshToken, Other: client.Other,
		})
	}
	file, err := os.CreateTemp("", "e5subbot-export-*.json")
	if err != nil {
		b.logger.Error("create account export", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not export accounts")
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(export); err != nil {
		b.logger.Error("encode account export", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not export accounts")
		return
	}
	if err := file.Close(); err != nil {
		b.logger.Error("close account export", zap.Error(err))
		b.send(message.Chat, "ERROR: Could not export accounts")
		return
	}
	b.send(message.Chat, &tb.Document{
		File: tb.FromDisk(file.Name()), FileName: strconv.FormatInt(message.Chat.ID, 10) + ".json", MIME: "application/json",
	})
}

func (b *Bot) onTask(message *tb.Message) {
	if !b.isAdmin(message.Chat.ID) {
		b.send(message.Chat, "WARN: Only admins can run this task")
		return
	}
	b.send(message.Chat, "Starting renewal task...")
	go b.RunTask(b.ctx)
}

func (b *Bot) onLog(message *tb.Message) {
	if !b.isAdmin(message.Chat.ID) {
		b.send(message.Chat, "WARN: Only admins can read logs")
		return
	}
	const path = "./log/latest.log"
	if _, err := os.Stat(path); err != nil {
		b.send(message.Chat, "No log file is available")
		return
	}
	b.send(message.Chat, &tb.Document{File: tb.FromDisk(path), FileName: "latest.log", MIME: "text/plain"})
}
