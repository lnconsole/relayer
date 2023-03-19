package main

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/lnconsole/relayer"
	proxy "github.com/lnconsole/relayer/conxole/proxy"
	"github.com/lnconsole/relayer/storage/postgresql"
	"github.com/nbd-wtf/go-nostr"
)

type Relay struct {
	PostgresDatabase string   `envconfig:"POSTGRESQL_DATABASE"`
	Whitelist        []string `envconfig:"WHITELIST"`
	PersistKinds     []int    `envconfig:"PERSIST_KINDS"`
	Relays           []string `envconfig:"RELAYS"`
	BotPubkey        string   `envconfig:"CONXOLE_BOT_PUBKEY"`
	Prod             bool     `envconfig:"PROD"`

	storage *postgresql.PostgresBackend
}

func (r *Relay) Name() string {
	return "Conxole Relay"
}

func (r *Relay) Storage() relayer.Storage {
	return r.storage
}

func (r *Relay) OnInitialized(*relayer.Server) {}

func (r *Relay) Init() error {
	// keep events for an hour only
	go func() {
		db := r.Storage().(*postgresql.PostgresBackend)

		for {
			intStrings := []string{}
			for _, k := range r.PersistKinds {
				intStrings = append(intStrings, strconv.Itoa(k))
			}
			param := "{" + strings.Join(intStrings, ",") + "}"
			db.Exec(
				`DELETE FROM event WHERE created_at < $1 AND NOT (kind = ANY($2::int[]))`,
				time.Now().Add(-60*time.Minute).Unix(),
				param,
			)
			time.Sleep(5 * time.Minute)
		}
	}()

	return nil
}

func (r *Relay) AcceptEvent(evt *nostr.Event) bool {
	// disallow anything from non-authorized pubkeys
	found := false
	for _, pubkey := range r.Whitelist {
		if pubkey == evt.PubKey {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	// block events that are too large
	jsonb, _ := json.Marshal(evt)

	return len(jsonb) <= 100000
}

func (r *Relay) BroadcastEvent(event nostr.Event) {
	// don't broadcast if this is not production
	if !r.Prod {
		return
	}
	if err := proxy.Broadcast(event); err != nil {
		log.Printf("broadcast error: %s", err)
	}
}

func (r *Relay) SubscribeEvents(filters nostr.Filters) {
	events, _ := proxy.Sub(filters)

	go func() {
		for em := range events {
			relayer.AddEvent(r, em.Event)
			relayer.NotifyListeners(&em.Event)
		}
	}()
}

func main() {
	// load env file
	if err := godotenv.Load(); err != nil {
		log.Fatalf("godotenv: %s", err)
	}
	r := Relay{}
	// store env vars in relay
	if err := envconfig.Process("", &r); err != nil {
		log.Fatalf("failed to read from env: %v", err)
	}
	for idx := range r.Relays {
		log.Printf("%s", r.Relays[idx])
	}
	// start relay
	r.storage = &postgresql.PostgresBackend{DatabaseURL: r.PostgresDatabase}
	if err := r.Storage().Init(); err != nil {
		log.Fatalf("storage init: %s", err)
	}
	if err := r.Init(); err != nil {
		log.Fatalf("relay init: %s", err)
	}
	// start proxy
	if err := proxy.Init(r.Relays); err != nil {
		log.Fatalf("server terminated: %v", err)
	}
	// define filters relevant to conxole
	now := time.Now()
	oneDay := time.Now().Add(-24 * time.Hour)
	filters := nostr.Filters{
		{
			Kinds: []int{
				nostr.KindSetMetadata, // 0
			},
			Since: &oneDay,
		},
		{
			Kinds: []int{
				nostr.KindTextNote,       // 1
				nostr.KindChannelMessage, // 42
				nostr.KindZap,            // 9735
			},
			Since: &now,
		},
		{
			Kinds: []int{
				nostr.KindEncryptedDirectMessage, // 4
			},
			Since: &now,
			Tags:  nostr.TagMap{"p": []string{r.BotPubkey}},
		},
	}
	// subscribe
	r.SubscribeEvents(filters)
	// start the server
	if err := relayer.Start(&r); err != nil {
		log.Fatalf("server terminated: %v", err)
	}
}
