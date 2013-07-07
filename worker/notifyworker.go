// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"fmt"

	"launchpad.net/tomb"

	// TODO: Use api/params.NotifyWatcher to avoid redeclaring it here
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
)

type notifyWorker struct {
	// Internal structure
	tomb tomb.Tomb
	// handler is what will handle when events are triggered
	handler WatchHandler
}

// NotifyWorker encapsulates the state logic for a worker which is based on a
// NotifyWatcher. We do a bit of setup, and then spin waiting for the watcher
// to trigger or for us to be killed, and then teardown cleanly.
type NotifyWorker interface {
	Wait() error
	Kill()
	// This is just Kill + Wait
	Stop() error
}

// WatchHandler implements the business logic that is triggered as part of
// watching
type WatchHandler interface {
	// Start the handler, this should create the watcher we will be waiting
	// on for more events. SetUp can return a Watcher even if there is an
	// error, and NotifyWorker will make sure to stop the Watcher.
	SetUp() (state.NotifyWatcher, error)
	// Cleanup any resources that are left around
	TearDown()
	// The Watcher has indicated there are changes, do whatever work is
	// necessary to process it
	Handle() error
}

func NewNotifyWorker(handler WatchHandler) NotifyWorker {
	nw := &notifyWorker{
		handler: handler,
	}
	go func() {
		defer nw.tomb.Done()
		nw.tomb.Kill(nw.loop())
	}()
	return nw
}

// Kill the loop with no-error
func (nw *notifyWorker) Kill() {
	nw.tomb.Kill(nil)
}

// Kill and Wait for this to exit
func (nw *notifyWorker) Stop() error {
	nw.tomb.Kill(nil)
	return nw.tomb.Wait()
}

// Wait for the looping to finish
func (nw *notifyWorker) Wait() error {
	return nw.tomb.Wait()
}

func (nw *notifyWorker) loop() error {
	// Replace calls to TearDown with a defer nw.handler.TearDown()
	var w state.NotifyWatcher
	var err error
	defer nw.handler.TearDown()
	if w, err = nw.handler.SetUp(); err != nil {
		if w != nil {
			// We don't bother to propogate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	if w == nil {
		return fmt.Errorf("SetUp returned a nil Watcher")
	}
	defer watcher.Stop(w, &nw.tomb)
	for {
		select {
		case <-nw.tomb.Dying():
			return tomb.ErrDying
		case <-w.Changes():
			if err := nw.handler.Handle(); err != nil {
				return err
			}
		}
	}
	panic("unreachable")
}
