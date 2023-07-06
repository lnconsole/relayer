package postgresql

import (
	"encoding/json"

	"github.com/fiatjaf/relayer/storage"
	"github.com/nbd-wtf/go-nostr"
)

func (b *PostgresBackend) SaveEvent(evt *nostr.Event) error {
	// react to different kinds of events
	if evt.Kind == nostr.KindSetMetadata || evt.Kind == nostr.KindContactList || (10000 <= evt.Kind && evt.Kind < 20000) {
		// delete past events from this user
		b.DB.Exec(`DELETE FROM event WHERE pubkey = $1 AND kind = $2 AND created_at < $3`, evt.PubKey, evt.Kind, evt.CreatedAt.Unix())
	} else if evt.Kind == nostr.KindRecommendServer {
		// delete past recommend_server events equal to this one
		b.DB.Exec(`DELETE FROM event WHERE pubkey = $1 AND kind = $2 AND content = $3 AND created_at < $4`,
			evt.PubKey, evt.Kind, evt.Content, evt.CreatedAt.Unix())
	} else if evt.Kind >= 30000 && evt.Kind < 40000 {
		// NIP-33
		d := evt.Tags.GetFirst([]string{"d"})
		b.DB.Exec(`DELETE FROM event WHERE pubkey = $1 AND kind = $2 AND created_at < $3 AND tagvalues && ARRAY[$4]`,
			evt.PubKey, evt.Kind, evt.CreatedAt.Unix(), d.Value())
	}

	// insert
	tagsj, _ := json.Marshal(evt.Tags)
	res, err := b.DB.Exec(`
        INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
    `, evt.ID, evt.PubKey, evt.CreatedAt.Unix(), evt.Kind, tagsj, evt.Content, evt.Sig)
	if err != nil {
		return err
	}

	nr, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if nr == 0 {
		return storage.ErrDupEvent
	}

	return nil
}

func (b *PostgresBackend) BeforeSave(evt *nostr.Event) {
	// do nothing
}

func (b *PostgresBackend) AfterSave(evt *nostr.Event) {
	// delete all but the 100 most recent ones for each key
	// b.DB.Exec(`DELETE FROM event WHERE pubkey = $1 AND kind = $2 AND created_at < (
	//   SELECT created_at FROM event WHERE pubkey = $1
	//   ORDER BY created_at DESC OFFSET 100 LIMIT 1
	// )`, evt.PubKey, evt.Kind)
}
