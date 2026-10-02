package config

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
)

type MySQL struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

type Config struct {
	BotToken      string
	Socks5        string
	BindMax       int
	Workers       int
	ErrorLimit    int
	Cron          string
	Notice        string
	Admins        []int64
	Database      string
	Table         string
	SQLitePath    string
	MySQL         MySQL
	LogStdoutOnly bool
}

type Manager struct {
	mu      sync.RWMutex
	current Config
	viper   *viper.Viper
}

func Load(path string) (*Manager, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetDefault("bindmax", 5)
	v.SetDefault("goroutine", 10)
	v.SetDefault("errlimit", 5)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg, err := decode(v)
	if err != nil {
		return nil, err
	}
	m := &Manager{current: cfg, viper: v}
	v.OnConfigChange(func(_ fsnotify.Event) {
		next, err := decode(v)
		if err != nil {
			log.Printf("ignoring invalid config update: %v", err)
			return
		}
		m.mu.Lock()
		m.current.BindMax = next.BindMax
		m.current.Workers = next.Workers
		m.current.ErrorLimit = next.ErrorLimit
		m.current.Notice = next.Notice
		m.current.Admins = next.Admins
		m.mu.Unlock()
	})
	v.WatchConfig()
	return m, nil
}

func (m *Manager) Current() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.current
	cfg.Admins = append([]int64(nil), cfg.Admins...)
	return cfg
}

func decode(v *viper.Viper) (Config, error) {
	cfg := Config{
		BotToken:      strings.TrimSpace(v.GetString("bot_token")),
		Socks5:        strings.TrimSpace(v.GetString("socks5")),
		BindMax:       v.GetInt("bindmax"),
		Workers:       v.GetInt("goroutine"),
		ErrorLimit:    v.GetInt("errlimit"),
		Cron:          strings.TrimSpace(v.GetString("cron")),
		Notice:        v.GetString("notice"),
		LogStdoutOnly: v.GetBool("log_stdout_only"),
		Database:      strings.ToLower(strings.TrimSpace(v.GetString("db"))),
		Table:         strings.TrimSpace(v.GetString("table")),
		SQLitePath:    strings.TrimSpace(v.GetString("sqlite.db")),
		MySQL: MySQL{
			Host:     strings.TrimSpace(v.GetString("mysql.host")),
			Port:     v.GetInt("mysql.port"),
			User:     v.GetString("mysql.user"),
			Password: v.GetString("mysql.password"),
			Database: v.GetString("mysql.database"),
		},
	}
	for _, raw := range strings.Split(v.GetString("admin"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return Config{}, fmt.Errorf("invalid admin ID %q", raw)
		}
		cfg.Admins = append(cfg.Admins, id)
	}
	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("bot_token is required")
	}
	if cfg.BindMax <= 0 || cfg.Workers <= 0 || cfg.ErrorLimit < 0 {
		return Config{}, fmt.Errorf("bindmax and goroutine must be positive; errlimit cannot be negative")
	}
	if cfg.Cron == "" {
		return Config{}, fmt.Errorf("cron is required")
	}
	if _, err := cron.ParseStandard(cfg.Cron); err != nil {
		return Config{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	if cfg.Table == "" {
		return Config{}, fmt.Errorf("table is required")
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(cfg.Table) {
		return Config{}, fmt.Errorf("table must contain only letters, numbers, and underscores")
	}
	switch cfg.Database {
	case "sqlite":
		if cfg.SQLitePath == "" {
			return Config{}, fmt.Errorf("sqlite.db is required")
		}
	case "mysql":
		if cfg.MySQL.Host == "" || cfg.MySQL.Port <= 0 || cfg.MySQL.User == "" || cfg.MySQL.Database == "" {
			return Config{}, fmt.Errorf("mysql host, port, user, and database are required")
		}
	default:
		return Config{}, fmt.Errorf("unsupported db %q", cfg.Database)
	}
	return cfg, nil
}
