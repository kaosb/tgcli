package store

import (
	"fmt"
	"strings"
	"time"
)

func (d *DB) UpsertChat(peerID, kind, name string, lastMsgTS time.Time) error {
	_, err := d.sql.Exec(`
		INSERT INTO chats(peer_id, kind, name, last_message_ts) VALUES(?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			kind=excluded.kind,
			name=COALESCE(NULLIF(excluded.name,''), chats.name),
			last_message_ts=MAX(chats.last_message_ts, excluded.last_message_ts)
	`, peerID, kind, name, unix(lastMsgTS))
	return err
}

func (d *DB) ListChats(kind string, limit int) ([]Chat, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT peer_id, kind, COALESCE(name,''), COALESCE(last_message_ts,0) FROM chats WHERE 1=1`
	var args []interface{}

	if strings.TrimSpace(kind) != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}

	query += " ORDER BY last_message_ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		var ts int64
		if err := rows.Scan(&c.PeerID, &c.Kind, &c.Name, &ts); err != nil {
			return nil, err
		}
		c.LastMessageTS = fromUnix(ts)
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

func (d *DB) GetChat(peerID string) (Chat, error) {
	row := d.sql.QueryRow(`SELECT peer_id, kind, COALESCE(name,''), COALESCE(last_message_ts,0) FROM chats WHERE peer_id = ?`, peerID)
	var c Chat
	var ts int64
	if err := row.Scan(&c.PeerID, &c.Kind, &c.Name, &ts); err != nil {
		return Chat{}, err
	}
	c.LastMessageTS = fromUnix(ts)
	return c, nil
}
