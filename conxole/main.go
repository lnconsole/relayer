package main

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"

	sm "github.com/SaveTheRbtz/generic-sync-map-go"
	"github.com/go-co-op/gocron"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/lnconsole/relayer"
	otelUptrace "github.com/lnconsole/relayer/conxole/otel"
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
	BeatzcoinPubkey  string   `envconfig:"BEATZCOIN_PUBKEY"`
	Prod             bool     `envconfig:"PROD"`

	storage *postgresql.PostgresBackend
	seen    sm.MapOf[string, struct{}]
}

func (r *Relay) Name() string {
	return "Conxole Relay"
}

func (r *Relay) Storage() relayer.Storage {
	return r.storage
}

func (r *Relay) OnInitialized(*relayer.Server) {}

func (r *Relay) Init() error {
	// prune kind0 events once a week
	s := gocron.NewScheduler(time.UTC)
	s.Cron("30 9 * * MON").Do(r.PruneProfiles)
	// s.Cron("* * * * *").Do(r.PruneProfiles)
	s.StartAsync()

	// keep events for an hour only
	go func() {
		db := r.Storage().(*postgresql.PostgresBackend)

		for {
			ctx, span := otel.Tracer("conxole-relay-tracer").Start(context.Background(), "delete-old-events-job")
			intStrings := []string{}
			for _, k := range r.PersistKinds {
				intStrings = append(intStrings, strconv.Itoa(k))
			}
			param := "{" + strings.Join(intStrings, ",") + "}"
			db.ExecContext(
				ctx,
				`DELETE FROM event WHERE created_at < $1 AND NOT (kind = ANY($2::int[]))`,
				time.Now().Add(-60*time.Minute).Unix(),
				param,
			)
			// temporary. needs refactor
			db.ExecContext(
				ctx,
				`DELETE FROM event WHERE kind = 42 AND created_at < $1`,
				time.Now().Add(-24*time.Hour).Unix(),
			)
			span.End()
			time.Sleep(5 * time.Minute)
		}
	}()
	// init seen map
	r.seen = sm.MapOf[string, struct{}]{}

	return nil
}

func (r *Relay) PruneProfiles() {
	ctx, span := otel.Tracer("conxole-relay-tracer").Start(context.Background(), "prune-profiles")
	defer span.End()

	// delete all profiles that are blacklisted or have activity count below threshold
	db := r.Storage().(*postgresql.PostgresBackend)
	db.ExecContext(ctx, "DELETE FROM event WHERE kind = 0 and (pubkey IN (SELECT pubkey FROM blacklist) OR pubkey NOT IN (SELECT pubkey FROM activity WHERE count > 1))")
}

func (r *Relay) AcceptEvent(evt *nostr.Event) bool {
	// disallow anything from non-authorized pubkeys
	// found := false
	// for _, pubkey := range r.Whitelist {
	// 	if pubkey == evt.PubKey {
	// 		found = true
	// 		break
	// 	}
	// }
	// if !found {
	// 	return false
	// }

	// reject if kind0 of the event is blacklisted
	var (
		db            = r.Storage().(*postgresql.PostgresBackend)
		ctx, span     = otel.Tracer("conxole-relay-tracer").Start(context.Background(), "accept-event")
		isBlacklisted bool
	)
	defer span.End()

	db.GetContext(
		ctx,
		&isBlacklisted,
		`SELECT EXISTS(SELECT * FROM blacklist WHERE pubkey = $1)`,
		evt.PubKey,
	)
	if isBlacklisted {
		return false
	}

	// block events that are too large
	jsonb, _ := json.Marshal(evt)

	return len(jsonb) <= 100000
}

func (r *Relay) BroadcastEvent(ctx context.Context, event nostr.Event) {
	// don't broadcast if this is not production
	if !r.Prod {
		return
	}
	// don't broadcast if it's a beatzcoin kind 33333
	if event.Kind == 33333 {
		return
	}
	if err := proxy.Broadcast(ctx, event); err != nil {
		log.Printf("broadcast error: %s", err)
	}
}

func (r *Relay) SubscribeEvents(ctx context.Context, filters nostr.Filters) {
	events, _ := proxy.Sub(ctx, filters)

	go func() {
		for em := range events {
			if _, ok := r.seen.LoadOrStore(em.Event.ID, struct{}{}); !ok {
				// if unseen, save and notify listeners
				relayer.AddEvent(ctx, r, em.Event)
				relayer.NotifyListeners(&em.Event)
			}
		}
	}()

	go func() {
		for {
			count := 0
			r.seen.Range(func(key string, value struct{}) bool {
				count += 1
				return true
			})
			log.Printf("size of seen: %d", count)
			time.Sleep(5 * time.Minute)
		}
	}()
}

func (r *Relay) FetchMetadataSync(ctx context.Context, filter nostr.Filter) []nostr.Event {
	return proxy.FetchMetadataSync(ctx, filter)
}

/*
Conxole -> Relay
- SubscribeEvents on hardcoded relays. When event is received from these relays, listeners are notified
- send "EVENT". if event is accepted, it'll be saved in db, and broadcasted to hardcoded relays

External Client -> Relay
- This shouldn't happen on prod, only on dev for testing
- send "EVENT". if event is accepted, it'll be saved in db, and listeners should be notified
- send "REQ". Should work like normal
*/
func main() {
	// load env file
	if err := godotenv.Load(); err != nil {
		log.Fatalf("godotenv: %s", err)
	}

	// uptrace
	otelUptrace.InitUptrace()
	defer func() {
		if err := otelUptrace.ShutdownUptrace(context.Background()); err != nil {
			log.Println(err)
		}
	}()

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
	nip90MergeDate := time.Date(2023, time.November, 1, 0, 0, 0, 0, time.UTC)
	filters := nostr.Filters{
		{
			Kinds: []int{
				nostr.KindSetMetadata,    // 0
				nostr.KindChannelMessage, // 42
				nostr.KindZap,            // 9735
				5000,                     // Request: Text Extraction
				5001,                     // Request: Summarization
				5002,                     // Request: Translation
				5100,                     // Request: Image Generation
				5101,                     // Request: Background Removal
				5102,                     // Request: Image Overlay
				5200,                     // Request: Video Conversion
				5201,                     // Request: Video Translation
				5300,                     // Request: Content Discovery
				5301,                     // Request: People Discovery
				5400,                     // Request: Event Count
				5500,                     // Request: Lightning Prism
				6000,                     // Result: Text Extraction
				6001,                     // Result: Summarization
				6002,                     // Result: Translation
				6100,                     // Result: Image Generation
				6101,                     // Result: Background Removal
				6102,                     // Result: Image Overlay
				6200,                     // Result: Video Conversion
				6201,                     // Result: Video Translation
				6300,                     // Result: Content Discovery
				6301,                     // Result: People Discovery
				6400,                     // Result: Event Count
				6500,                     // Result: Lightning Prism
				7000,                     // Job Feedback
				31990,                    // NIP-89 Application Handler

				65000, // Legacy Job Feedback
				65001, // Legacy Job Result
				65005, // Legacy Image Generation
				65007, // Legacy Background Removal
				65008, // Legacy Overlay Images
				65009, // Legacy Lightning Prism
			},
			Since: &now,
		},
		{
			Kinds: []int{
				nostr.KindTextNote,               // 1
				nostr.KindEncryptedDirectMessage, // 4
			},
			Since: &now,
			Tags:  nostr.TagMap{"p": []string{r.BotPubkey}},
		},
		{
			Kinds: []int{
				nostr.KindEncryptedDirectMessage, // 4
			},
			Since: &now,
			Tags:  nostr.TagMap{"p": []string{r.BeatzcoinPubkey}},
		},
		{
			Kinds: []int{
				65001, // Legacy Job Result Kind
			},
			Until: &nip90MergeDate,
		},
	}
	// subscribe
	r.SubscribeEvents(context.Background(), filters)
	// start the server
	if err := relayer.Start(&r); err != nil {
		log.Fatalf("server terminated: %v", err)
	}
}
