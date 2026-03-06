package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	up      func(*DB) error
}

var schemaMigrations = []migration{
	{version: 1, name: "core schema", up: migrateCoreSchema},
	{version: 2, name: "messages fts", up: migrateMessagesFTS},
}

func (d *DB) ensureSchema() error {
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	applied := map[int]bool{}
	rows, err := d.sql.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("scan migration: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range schemaMigrations {
		if applied[m.version] {
			continue
		}
		if err := m.up(d); err != nil {
			return fmt.Errorf("migration %03d %s: %w", m.version, m.name, err)
		}
		if _, err := d.sql.Exec(
			`INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)`,
			m.version, m.name, time.Now().UTC().Unix(),
		); err != nil {
			return fmt.Errorf("record migration %03d: %w", m.version, err)
		}
	}
	return nil
}

func migrateCoreSchema(d *DB) error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			peer_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			name TEXT,
			last_message_ts INTEGER
		);

		CREATE TABLE IF NOT EXISTS messages (
			rowid INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id TEXT NOT NULL,
			chat_name TEXT,
			msg_id INTEGER NOT NULL,
			sender_id TEXT,
			sender_name TEXT,
			ts INTEGER NOT NULL,
			from_me INTEGER NOT NULL,
			text TEXT,
			media_type TEXT,
			media_caption TEXT,
			filename TEXT,
			mime_type TEXT,
			file_id TEXT,
			file_size INTEGER,
			local_path TEXT,
			UNIQUE(chat_id, msg_id),
			FOREIGN KEY (chat_id) REFERENCES chats(peer_id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_id, ts);
		CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(ts);
	`)
	if err != nil {
		return fmt.Errorf("create tables: %w", err)
	}
	return nil
}

func migrateMessagesFTS(d *DB) error {
	ftsExists, err := d.tableExists("messages_fts")
	if err != nil {
		return err
	}

	if !ftsExists {
		if _, err := d.sql.Exec(`
			CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
				text,
				media_caption,
				filename,
				chat_name,
				sender_name
			)
		`); err != nil {
			d.ftsEnabled = false
			return nil
		}
	}

	// Triggers to keep FTS in sync.
	if _, err := d.sql.Exec(`
		DROP TRIGGER IF EXISTS messages_ai;
		DROP TRIGGER IF EXISTS messages_ad;
		DROP TRIGGER IF EXISTS messages_au;

		CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, text, media_caption, filename, chat_name, sender_name)
			VALUES (new.rowid, COALESCE(new.text,''), COALESCE(new.media_caption,''), COALESCE(new.filename,''), COALESCE(new.chat_name,''), COALESCE(new.sender_name,''));
		END;

		CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
			DELETE FROM messages_fts WHERE rowid = old.rowid;
		END;

		CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
			DELETE FROM messages_fts WHERE rowid = old.rowid;
			INSERT INTO messages_fts(rowid, text, media_caption, filename, chat_name, sender_name)
			VALUES (new.rowid, COALESCE(new.text,''), COALESCE(new.media_caption,''), COALESCE(new.filename,''), COALESCE(new.chat_name,''), COALESCE(new.sender_name,''));
		END;
	`); err != nil {
		d.ftsEnabled = false
		return nil
	}

	if !ftsExists {
		// Backfill existing messages into FTS.
		if _, err := d.sql.Exec(`
			INSERT INTO messages_fts(rowid, text, media_caption, filename, chat_name, sender_name)
			SELECT rowid, COALESCE(text,''), COALESCE(media_caption,''), COALESCE(filename,''), COALESCE(chat_name,''), COALESCE(sender_name,'')
			FROM messages
		`); err != nil {
			d.ftsEnabled = false
			return nil
		}
	}

	d.ftsEnabled = true
	return nil
}

func (d *DB) tableExists(table string) (bool, error) {
	row := d.sql.QueryRow(`SELECT 1 FROM sqlite_master WHERE name = ? AND type IN ('table','view')`, table)
	var one int
	if err := row.Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *DB) tableHasColumn(table, column string) (bool, error) {
	rows, err := d.sql.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid     int
			name    string
			colType string
			notNull int
			pk      int
			dflt    sql.NullString
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}
