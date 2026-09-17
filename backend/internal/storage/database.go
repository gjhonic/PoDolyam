package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open действительно проверяет соединение: создание pgxpool само по себе БД не проверяет.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("конфигурация БД: %w", err)
	}
	cfg.MaxConns = 8
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}
	return db, nil
}

// Migrate применяет только транзакционные SQL-миграции.
// Advisory lock сериализует запуски, а checksum запрещает переписывать применённый SQL.
func Migrate(ctx context.Context, db *pgxpool.Pool, files fs.FS) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(73642001)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	rows, err := tx.Query(ctx, "SELECT name,checksum FROM schema_migrations ORDER BY name")
	if err != nil {
		return err
	}
	applied := map[string]string{}
	history := []string{}
	for rows.Next() {
		var n, c string
		if err = rows.Scan(&n, &c); err != nil {
			rows.Close()
			return err
		}
		applied[n] = c
		history = append(history, n)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(history) > len(names) {
		return fmt.Errorf("история миграций новее приложения")
	}
	for i, n := range history {
		if names[i] != n {
			return fmt.Errorf("изменён порядок миграций")
		}
	}
	for _, name := range names {
		sql, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(sql)
		checksum := hex.EncodeToString(sum[:])
		if old, ok := applied[name]; ok {
			if old != checksum {
				return fmt.Errorf("изменена миграция %s", name)
			}
			continue
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("миграция %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", name, checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
