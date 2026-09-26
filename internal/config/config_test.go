package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKeepsExistingConfigKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `bot_token: test-token
bindmax: 7
goroutine: 3
errlimit: 2
admin: "101, 202"
cron: "1 */1 * * *"
db: sqlite
table: users
sqlite:
  db: data.db
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := manager.Current()
	if cfg.BindMax != 7 || cfg.Workers != 3 || cfg.ErrorLimit != 2 || len(cfg.Admins) != 2 || cfg.Admins[1] != 202 || cfg.Table != "users" {
		t.Fatalf("existing config was not decoded: %+v", cfg)
	}
	cfg.Admins[0] = 999
	if manager.Current().Admins[0] != 101 {
		t.Fatal("config snapshot exposes mutable admin state")
	}
}

func TestLoadRejectsUnsafeTableName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `bot_token: test-token
cron: "1 */1 * * *"
db: sqlite
table: "users; DROP TABLE users"
sqlite:
  db: data.db
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "table") {
		t.Fatalf("unsafe table name accepted: %v", err)
	}
}
