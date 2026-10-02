package bot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Debcharon/E5SubBot/internal/renewal"
	"go.uber.org/zap"
	tb "gopkg.in/tucnak/telebot.v2"
)

func (b *Bot) RunTask(ctx context.Context) error {
	b.taskMu.Lock()
	if b.stopping {
		b.taskMu.Unlock()
		return context.Canceled
	}
	b.taskWG.Add(1)
	b.taskMu.Unlock()
	defer b.taskWG.Done()
	report, err := b.runner.Run(ctx)
	if errors.Is(err, renewal.ErrAlreadyRunning) {
		b.logger.Info("renewal task skipped because another task is running")
		return err
	}
	if err != nil {
		b.logger.Error("renewal task failed", zap.Error(err))
		for _, adminID := range b.settings.Current().Admins {
			b.sendToID(adminID, "Renewal task failed: "+err.Error())
		}
		return err
	}
	failedByUser := make(map[int64]int)
	totalByUser := make(map[int64]int)
	var failed, authorizationRequired int
	wrongUsers := make(map[int64]bool)
	authorizationUsers := make(map[int64]bool)
	detailsByUser := make(map[int64][]string)
	for _, result := range report.Results {
		if err := ctx.Err(); err != nil {
			return err
		}
		userID := result.Client.TelegramID
		totalByUser[userID]++
		if result.Err == nil {
			continue
		}
		failed++
		failedByUser[userID]++
		wrongUsers[userID] = true
		b.logger.Warn("account renewal failed", zap.Int("account_id", result.Client.ID), zap.Error(result.Err))
		detail := fmt.Sprintf("%s: %v", result.Client.Alias, result.Err)
		if result.NeedsAuthorization {
			authorizationRequired++
			authorizationUsers[userID] = true
			detail += " (authorization or updated credentials required)"
		}
		detailsByUser[userID] = append(detailsByUser[userID], detail)
	}
	for userID, total := range totalByUser {
		feedback := fmt.Sprintf("Task feedback\nTime: %s\nResult: %d/%d", time.Now().Format("2006-01-02 15:04:05"), total-failedByUser[userID], total)
		if details := detailsByUser[userID]; len(details) > 0 {
			feedback += "\n" + strings.Join(details, "\n")
		}
		if authorizationUsers[userID] {
			feedback += "\nAccount data retained. Back up with /export, then use /unbind and /bind to renew authorization."
		}
		b.sendToID(userID, limitMessage(feedback))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	var wrongIDs []string
	for id := range wrongUsers {
		wrongIDs = append(wrongIDs, strconv.FormatInt(id, 10))
	}
	sort.Strings(wrongIDs)
	var authorizationIDs []string
	for id := range authorizationUsers {
		authorizationIDs = append(authorizationIDs, strconv.FormatInt(id, 10))
	}
	sort.Strings(authorizationIDs)
	adminMessage := fmt.Sprintf("Task feedback (administrator)\nComplete time: %s\nTime cost: %.2fs\nResult: %d/%d\nFailed users: %s\nAuthorization required: %s",
		time.Now().Format("2006-01-02 15:04:05"), report.Duration.Seconds(),
		len(report.Results)-failed, len(report.Results), strings.Join(wrongIDs, ", "), strings.Join(authorizationIDs, ", "))
	for _, adminID := range b.settings.Current().Admins {
		b.sendToID(adminID, limitMessage(adminMessage))
	}
	b.logger.Info("renewal task completed", zap.Int("total", len(report.Results)), zap.Int("failed", failed), zap.Int("authorization_required", authorizationRequired))
	return nil
}

func limitMessage(message string) string {
	// Telegram counts UTF-16 code units; 1,900 runes fit even with supplementary characters.
	runes := []rune(message)
	if len(runes) > 1900 {
		return string(runes[:1900]) + "\n[truncated]"
	}
	return message
}

func (b *Bot) sendToID(userID int64, message interface{}, options ...interface{}) {
	b.send(&tb.Chat{ID: userID}, message, options...)
}
