package app

import (
	"context"
	"fmt"
	"os"

	"github.com/Debcharon/E5SubBot/internal/account"
	"github.com/Debcharon/E5SubBot/internal/bot"
	"github.com/Debcharon/E5SubBot/internal/config"
	"github.com/Debcharon/E5SubBot/internal/microsoft"
	"github.com/Debcharon/E5SubBot/internal/renewal"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func Run(ctx context.Context) error {
	settings, err := config.Load("config.yml")
	if err != nil {
		return err
	}
	logger, err := newLogger()
	if err != nil {
		return err
	}
	defer logger.Sync()
	accounts, err := account.Open(settings.Current())
	if err != nil {
		return err
	}
	defer accounts.Close()
	ms := microsoft.NewClient(nil)
	runner := renewal.New(accounts, ms, settings)
	telegram, err := bot.New(ctx, settings, accounts, ms, runner, logger)
	if err != nil {
		return err
	}
	scheduler := cron.New()
	if _, err := scheduler.AddFunc(settings.Current().Cron, func() { telegram.RunTask(ctx) }); err != nil {
		return fmt.Errorf("schedule renewal: %w", err)
	}
	scheduler.Start()
	go telegram.Start()
	<-ctx.Done()
	telegram.Stop()
	<-scheduler.Stop().Done()
	return nil
}

func newLogger() (*zap.Logger, error) {
	if err := os.MkdirAll("./log", 0700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file := &lumberjack.Logger{
		Filename: "./log/latest.log", MaxSize: 1, MaxBackups: 5,
		MaxAge: 30, Compress: true,
	}
	encoder := zap.NewProductionEncoderConfig()
	encoder.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoder),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(file)),
		zapcore.InfoLevel)
	return zap.New(core, zap.AddCaller()), nil
}
