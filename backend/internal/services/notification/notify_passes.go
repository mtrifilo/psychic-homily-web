package notification

import (
	"errors"
	"fmt"
	"log"
	"runtime/debug"
)

// Pass names, as they appear in MatchAndNotify's returned error and its logs.
const (
	notifyPassFilter       = "filter"
	notifyPassArtistFollow = "artist-follow"
	notifyPassVenueFollow  = "venue-follow"
	notifyPassSceneFollow  = "scene-follow"
)

// notifyPass is one fanout inside MatchAndNotify.
type notifyPass struct {
	name string
	run  func() error
}

// notifyPassPanic is the value runNotifyPasses re-raises when a pass panicked.
// It names the pass and carries the stack captured where the panic was
// recovered, because the re-raise happens after the remaining passes have run
// and a stack taken by the caller's recover would point here instead.
type notifyPassPanic struct {
	pass   string
	showID uint
	value  any
	stack  []byte
}

func (p *notifyPassPanic) Error() string {
	return fmt.Sprintf("%s pass panicked for show %d: %v\n\n%s", p.pass, p.showID, p.value, p.stack)
}

// Unwrap exposes the original panic value when it was an error.
func (p *notifyPassPanic) Unwrap() error {
	err, _ := p.value.(error)
	return err
}

// runNotifyPasses runs every pass in order; no pass's failure or panic stops
// the passes after it.
//
// Errors are logged with the pass name and show id, wrapped with the pass name,
// and returned joined (nil when every pass succeeded).
//
// Panics stay fatal to the call: each one is recovered and logged so the
// remaining passes still run, and the first is re-raised as a *notifyPassPanic
// once they have. The caller's own recover (GoSafe, the scheduled loop) then
// handles it as it handles any other panic.
func runNotifyPasses(showID uint, passes []notifyPass) error {
	var errs []error
	var firstPanic *notifyPassPanic
	for _, p := range passes {
		panicked, err := runNotifyPass(showID, p)
		if panicked != nil {
			log.Printf("notification match: %s pass panicked for show %d: %v\n%s",
				p.name, showID, panicked.value, panicked.stack)
			if firstPanic == nil {
				firstPanic = panicked
			}
			continue
		}
		if err != nil {
			log.Printf("notification match: %s pass failed for show %d: %v", p.name, showID, err)
			errs = append(errs, fmt.Errorf("%s pass: %w", p.name, err))
		}
	}
	if firstPanic != nil {
		panic(firstPanic)
	}
	return errors.Join(errs...)
}

// runNotifyPass runs one pass, converting a panic into a *notifyPassPanic.
func runNotifyPass(showID uint, p notifyPass) (panicked *notifyPassPanic, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = &notifyPassPanic{pass: p.name, showID: showID, value: r, stack: debug.Stack()}
		}
	}()
	return nil, p.run()
}
