package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Debcharon/E5SubBot/internal/renewal"
	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
)

func (b *Bot) RunTask(ctx context.Context) {
	report, err := b.runner.Run(ctx)
	if errors.Is(err, renewal.ErrAlreadyRunning) {
		b.logger.Info("renewal task skipped because another task is running")
		return
	}
	if err != nil {
		b.logger.Error("renewal task failed", zap.Error(err))
		for _, adminID := range b.settings.Current().Admins {
			b.sendToID(adminID, "Renewal task failed: "+err.Error())
		}
		return
	}
	failedByUser := make(map[int64]int)
	totalByUser := make(map[int64]int)
	var failed, removed int
	var wrongUsers, removedUsers []string
	for _, result := range report.Results {
		userID := result.Client.TelegramID
		totalByUser[userID]++
		if result.Err == nil {
			continue
		}
		failed++
		failedByUser[userID]++
		wrongUsers = append(wrongUsers, strconv.FormatInt(userID, 10))
		b.logger.Warn("account renewal failed", zap.Int("account_id", result.Client.ID), zap.Error(result.Err))
		if result.Removed {
			removed++
			removedUsers = append(removedUsers, strconv.FormatInt(userID, 10))
			b.sendToID(userID, fmt.Sprintf("Your account was automatically unbound after repeated errors.\nAlias: %s\nclient_id: %s\nclient_secret: %s",
				result.Client.Alias, result.Client.ClientID, result.Client.ClientSecret))
			continue
		}
		button := tb.InlineButton{Unique: "account-unbind", Text: "Click to unbind", Data: strconv.Itoa(result.Client.ID)}
		b.sendToID(userID, fmt.Sprintf("An error occurred while renewing account %s.\nError: %v", result.Client.Alias, result.Err),
			&tb.ReplyMarkup{InlineKeyboard: [][]tb.InlineButton{{button}}})
	}
	for userID, total := range totalByUser {
		b.sendToID(userID, fmt.Sprintf("Task feedback\nTime: %s\nResult: %d/%d",
			time.Now().Format("2006-01-02 15:04:05"), total-failedByUser[userID], total))
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	adminMessage := fmt.Sprintf("Task feedback (administrator)\nComplete time: %s\nTime cost: %.2fs\nResult: %d/%d\nWrong: %s\nCleared: %s",
		time.Now().Format("2006-01-02 15:04:05"), report.Duration.Seconds(),
		len(report.Results)-failed, len(report.Results), strings.Join(wrongUsers, ", "), strings.Join(removedUsers, ", "))
	for _, adminID := range b.settings.Current().Admins {
		b.sendToID(adminID, adminMessage)
	}
	b.logger.Info("renewal task completed", zap.Int("total", len(report.Results)), zap.Int("failed", failed), zap.Int("removed", removed))
}

func (b *Bot) sendToID(userID int64, message interface{}, options ...interface{}) {
	chat, err := b.api.ChatByID(strconv.FormatInt(userID, 10))
	if err != nil {
		b.logger.Warn("resolve Telegram chat", zap.Int64("user_id", userID), zap.Error(err))
		return
	}
	b.send(chat, message, options...)
}
