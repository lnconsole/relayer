package relayer

import (
	"context"
	"fmt"

	"github.com/lnconsole/relayer/storage"
	"github.com/nbd-wtf/go-nostr"
)

func AddEvent(ctx context.Context, relay Relay, evt nostr.Event) (accepted bool, message string) {
	ctx, span := relay.Tracer().Start(ctx, "AddEvent")
	defer span.End()

	store := relay.Storage()
	advancedSaver, _ := store.(AdvancedSaver)

	if 20000 <= evt.Kind && evt.Kind < 30000 {
		// do not store ephemeral events
	} else {
		if advancedSaver != nil {
			advancedSaver.BeforeSave(&evt)
		}

		if saveErr := store.SaveEvent(ctx, &evt); saveErr != nil {
			switch saveErr {
			case storage.ErrDupEvent:
				return true, saveErr.Error()
			default:
				return false, fmt.Sprintf("error: failed to save: %s", saveErr.Error())
			}
		}

		if advancedSaver != nil {
			advancedSaver.AfterSave(&evt)
		}
	}

	return true, ""
}
