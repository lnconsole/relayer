package relayproxy

import (
	"context"
	"log"
	"sync"

	"github.com/nbd-wtf/go-nostr"
)

type Unsub func()

var (
	relays map[string]*nostr.Relay
)

func Init(destRelays []string) error {
	log.Printf("proxy init")

	relays = make(map[string]*nostr.Relay)

	for idx := range destRelays {
		url := destRelays[idx]
		relay, err := nostr.RelayConnect(context.Background(), url)
		if err != nil {
			log.Printf("failed to connect to %s: %s", url, err)
			continue
		}
		relays[url] = relay

		go func() {
			for notice := range relay.Notices {
				log.Printf("(%s) notice: %s", relay.URL, notice)
			}
		}()
	}

	return nil
}

func Sub(filters nostr.Filters) (stream chan nostr.EventMessage, cleanup Unsub) {
	stream = make(chan nostr.EventMessage)
	unsub := make(chan struct{})

	if len(relays) == 0 {
		log.Print("no relays provided")
		return stream, func() { gracefulCleanup(unsub) }
	}

	for _, relay := range relays {
		sub := relay.Subscribe(context.Background(), filters)

		go func(sub *nostr.Subscription) {
			for evt := range sub.Events {
				if evt != nil {
					stream <- nostr.EventMessage{Relay: relay.URL, Event: *evt}
				} else {
					log.Printf("why are we getting nil event")
				}
			}
		}(sub)

		go func() {
			<-unsub
			sub.Unsub()
		}()
	}

	log.Printf("-_-_-_ subscribed to filters: %v", filters)

	return stream, func() { gracefulCleanup(unsub) }
}

func gracefulCleanup(c chan struct{}) {
	select {
	case <-c:
		close(c)
	default:
		close(c)
	}
}

func Broadcast(event nostr.Event) error {
	var (
		wg  sync.WaitGroup
		ctx = context.Background()
	)
	for idx := range relays {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			relay, err := nostr.RelayConnect(ctx, url)
			if err != nil {
				log.Printf("failed to connect to %s: %s", url, err)
				return
			}
			log.Printf("posting to: %s, %s", url, relay.Publish(ctx, event))
			relay.Close()
		}(relays[idx].URL)
	}
	wg.Wait()

	return nil
}
