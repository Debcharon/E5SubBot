package account

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Debcharon/E5SubBot/internal/config"
)

func TestLegacySQLiteTableAndOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE users (
		id integer PRIMARY KEY AUTOINCREMENT,
		tg_id integer NOT NULL, refresh_token text NOT NULL, ms_id text NOT NULL,
		uptime integer NOT NULL, alias text NOT NULL, client_id text NOT NULL,
		client_secret text NOT NULL, other text
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO users (tg_id, refresh_token, ms_id, uptime, alias, client_id, client_secret, other)
		VALUES (101, 'old-refresh', 'legacy-id', 1000, 'legacy', 'app-id', 'secret', NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err := Open(config.Config{Database: "sqlite", SQLitePath: path, Table: "users"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	clients, err := repo.ListByUser(ctx, 101)
	if err != nil || len(clients) != 1 || clients[0].RefreshToken != "old-refresh" || clients[0].Other != "" {
		t.Fatalf("legacy row not preserved: clients=%+v err=%v", clients, err)
	}
	if _, err := repo.GetForUser(ctx, clients[0].ID, 202); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another user should not read account: %v", err)
	}
	if deleted, err := repo.DeleteForUser(ctx, clients[0].ID, 202); err != nil || deleted {
		t.Fatalf("another user should not delete account: deleted=%v err=%v", deleted, err)
	}
	client := clients[0]
	client.RefreshToken = "rotated-refresh"
	client.Alias = "stale snapshot"
	client.ClientSecret = "stale secret"
	if err := repo.Update(ctx, &client); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.GetForUser(ctx, client.ID, 101)
	if err != nil || updated.RefreshToken != "rotated-refresh" || updated.Alias != "legacy" || updated.ClientSecret != "secret" || updated.UpdatedAtUnix != 1000 {
		t.Fatalf("token rotation was not saved: client=%+v err=%v", updated, err)
	}
	if deleted, err := repo.DeleteForUser(ctx, client.ID, 101); err != nil || !deleted {
		t.Fatalf("owner could not delete account: deleted=%v err=%v", deleted, err)
	}
}

func TestCreateAndExists(t *testing.T) {
	repo, err := Open(config.Config{Database: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "new.db"), Table: "users"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	client := &Client{TelegramID: 101, RefreshToken: "refresh", MicrosoftID: "id", Alias: "first", ClientID: "app", ClientSecret: "secret"}
	if err := repo.Create(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client.ID == 0 || client.UpdatedAtUnix == 0 {
		t.Fatalf("new account missing generated fields: %+v", client)
	}
	exists, err := repo.Exists(ctx, 101, "app")
	if err != nil || !exists {
		t.Fatalf("new account not found: exists=%v err=%v", exists, err)
	}
	exists, err = repo.Exists(ctx, 202, "app")
	if err != nil || exists {
		t.Fatalf("account leaked to another user: exists=%v err=%v", exists, err)
	}
}
