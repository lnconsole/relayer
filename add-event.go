package relayer

import (
	"fmt"

	"github.com/fiatjaf/relayer/storage"
	"github.com/nbd-wtf/go-nostr"
)

func AddEvent(relay Relay, evt nostr.Event) (accepted bool, message string) {
	store := relay.Storage()
	advancedSaver, _ := store.(AdvancedSaver)

	if !relay.AcceptEvent(&evt) {
		return false, "blocked: event blocked by relay"
	}

	if 20000 <= evt.Kind && evt.Kind < 30000 {
		// do not store ephemeral events
	} else {
		if advancedSaver != nil {
			advancedSaver.BeforeSave(&evt)
		}

		if saveErr := store.SaveEvent(&evt); saveErr != nil {
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

	notifyListeners(&evt)

	return true, ""
}
