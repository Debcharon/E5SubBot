package account

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/Debcharon/E5SubBot/internal/config"
	mysql "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// Client retains the existing table columns and JSON-independent field names.
type Client struct {
	ID            int
	TelegramID    int64
	RefreshToken  string
	MicrosoftID   string
	UpdatedAtUnix int64
	Alias         string
	ClientID      string
	ClientSecret  string
	Other         string
}

type Repository struct {
	db    *sql.DB
	table string
}

const columns = "id, tg_id, refresh_token, ms_id, uptime, alias, client_id, client_secret, `other`"

func Open(cfg config.Config) (*Repository, error) {
	var driver, source string
	switch cfg.Database {
	case "sqlite":
		driver, source = "sqlite", cfg.SQLitePath
	case "mysql":
		driver = "mysql"
		source = (&mysql.Config{
			User: cfg.MySQL.User, Passwd: cfg.MySQL.Password, Net: "tcp",
			Addr:   net.JoinHostPort(cfg.MySQL.Host, strconv.Itoa(cfg.MySQL.Port)),
			DBName: cfg.MySQL.Database, ParseTime: true, Loc: time.Local,
			Params:  map[string]string{"charset": "utf8mb4"},
			Timeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second,
		}).FormatDSN()
	default:
		return nil, fmt.Errorf("unsupported database %q", cfg.Database)
	}
	db, err := sql.Open(driver, source)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if cfg.Database == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(3 * time.Minute)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	r := &Repository{db: db, table: "`" + cfg.Table + "`"}
	if err := r.createTable(cfg.Database); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repository) createTable(driver string) error {
	var schema string
	if driver == "sqlite" {
		schema = "id INTEGER PRIMARY KEY AUTOINCREMENT, tg_id INTEGER NOT NULL, refresh_token TEXT NOT NULL, ms_id TEXT NOT NULL, uptime INTEGER NOT NULL, alias TEXT NOT NULL, client_id TEXT NOT NULL, client_secret TEXT NOT NULL, `other` TEXT"
	} else {
		schema = "id BIGINT AUTO_INCREMENT PRIMARY KEY, tg_id BIGINT NOT NULL, refresh_token LONGTEXT NOT NULL, ms_id LONGTEXT NOT NULL, uptime BIGINT NOT NULL, alias LONGTEXT NOT NULL, client_id LONGTEXT NOT NULL, client_secret LONGTEXT NOT NULL, `other` LONGTEXT"
	}
	if _, err := r.db.Exec("CREATE TABLE IF NOT EXISTS " + r.table + " (" + schema + ")"); err != nil {
		return fmt.Errorf("create account table: %w", err)
	}
	return nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Create(ctx context.Context, client *Client) error {
	client.UpdatedAtUnix = time.Now().Unix()
	query := "INSERT INTO " + r.table + " (tg_id, refresh_token, ms_id, uptime, alias, client_id, client_secret, `other`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
	result, err := r.db.ExecContext(ctx, query, client.TelegramID, client.RefreshToken, client.MicrosoftID, client.UpdatedAtUnix,
		client.Alias, client.ClientID, client.ClientSecret, client.Other)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err == nil {
		client.ID = int(id)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, client *Client) error {
	query := "UPDATE " + r.table + " SET refresh_token = ?, uptime = ? WHERE id = ? AND tg_id = ?"
	_, err := r.db.ExecContext(ctx, query, client.RefreshToken, client.UpdatedAtUnix, client.ID, client.TelegramID)
	return err
}

func (r *Repository) DeleteForUser(ctx context.Context, id int, userID int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM "+r.table+" WHERE id = ? AND tg_id = ?", id, userID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *Repository) GetForUser(ctx context.Context, id int, userID int64) (Client, error) {
	var client Client
	query := "SELECT " + columns + " FROM " + r.table + " WHERE id = ? AND tg_id = ?"
	err := scanClient(r.db.QueryRowContext(ctx, query, id, userID), &client)
	return client, err
}

func (r *Repository) ListByUser(ctx context.Context, userID int64) ([]Client, error) {
	return r.list(ctx, "SELECT "+columns+" FROM "+r.table+" WHERE tg_id = ?", userID)
}

func (r *Repository) ListAll(ctx context.Context) ([]Client, error) {
	return r.list(ctx, "SELECT "+columns+" FROM "+r.table)
}

func (r *Repository) list(ctx context.Context, query string, args ...any) ([]Client, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clients []Client
	for rows.Next() {
		var client Client
		if err := scanClient(rows, &client); err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanClient(row scanner, client *Client) error {
	var other sql.NullString
	err := row.Scan(&client.ID, &client.TelegramID, &client.RefreshToken, &client.MicrosoftID, &client.UpdatedAtUnix,
		&client.Alias, &client.ClientID, &client.ClientSecret, &other)
	client.Other = other.String
	return err
}

func (r *Repository) Exists(ctx context.Context, userID int64, clientID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+r.table+" WHERE tg_id = ? AND client_id = ?", userID, clientID).Scan(&count)
	return count > 0, err
}
