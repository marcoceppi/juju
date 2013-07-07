// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"fmt"
	"sync"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
)

var shortWait = 5 * time.Millisecond
var longWait = 500 * time.Millisecond

type notifyWorkerSuite struct {
	coretesting.LoggingSuite
	worker worker.NotifyWorker
	actor  *ActionsHandler
}

var _ = gc.Suite(&notifyWorkerSuite{})

func (s *notifyWorkerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.actor = &ActionsHandler{
		actions: nil,
		handled: make(chan struct{}),
		watcher: &TestWatcher{
			out: make(chan struct{}),
		},
	}
	s.worker = worker.NewNotifyWorker(s.actor)
}

func (s *notifyWorkerSuite) TearDownTest(c *gc.C) {
	s.stopWorker(c)
	s.LoggingSuite.TearDownTest(c)
}

type ActionsHandler struct {
	actions []string
	mu      sync.Mutex
	// Signal handled when we get a handle() call
	handled      chan struct{}
	setupError   error
	handlerError error
	watcher      *TestWatcher
}

func (a *ActionsHandler) SetUp() (state.NotifyWatcher, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "setup")
	if a.watcher == nil {
		return nil, a.setupError
	}
	return a.watcher, a.setupError
}

func (a *ActionsHandler) TearDown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "teardown")
}

func (a *ActionsHandler) Handle() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, "handler")
	if a.handled != nil {
		a.handled <- struct{}{}
	}
	return a.handlerError
}

func (a *ActionsHandler) CheckActions(c *gc.C, actions ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c.Check(a.actions, gc.DeepEquals, actions)
}

// During teardown we try to stop the worker, but don't hang the test suite if
// Stop never returns
func (s *notifyWorkerSuite) stopWorker(c *gc.C) {
	if s.worker == nil {
		return
	}
	done := make(chan error)
	go func() {
		done <- s.worker.Stop()
	}()
	select {
	case err := <-done:
		c.Check(err, gc.IsNil)
	case <-time.After(longWait):
		c.Errorf("Failed to stop worker after %.3fs", longWait.Seconds())
	}
	s.actor = nil
	s.worker = nil
}

type TestWatcher struct {
	mu        sync.Mutex
	out       chan struct{}
	action    chan struct{}
	stopped   bool
	stopError error
}

func (tw *TestWatcher) Changes() <-chan struct{} {
	return tw.out
}

func (tw *TestWatcher) Err() error {
	return nil
}

func (tw *TestWatcher) Stop() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	tw.stopped = true
	return tw.stopError
}

func (tw *TestWatcher) SetStopError(err error) {
	tw.mu.Lock()
	tw.stopError = err
	tw.mu.Unlock()
}

func (tw *TestWatcher) TriggerChange(c *gc.C) {
	select {
	case tw.out <- struct{}{}:
	case <-time.After(longWait):
		c.Errorf("Timed out triggering change after %.3fs", longWait.Seconds())
	}
}

func WaitShort(c *gc.C, w worker.NotifyWorker) error {
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(shortWait):
		c.Errorf("Wait() failed to return after %.3fs", shortWait.Seconds())
	}
	return nil
}

func WaitForHandled(c *gc.C, handled chan struct{}) {
	select {
	case <-handled:
		return
	case <-time.After(longWait):
		c.Errorf("handled failed to signal after", longWait.Seconds())
	}
}

func (s *notifyWorkerSuite) TestKill(c *gc.C) {
	s.worker.Kill()
	err := WaitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestStop(c *gc.C) {
	err := s.worker.Stop()
	c.Assert(err, gc.IsNil)
	// After stop, Wait should return right away
	err = WaitShort(c, s.worker)
	c.Assert(err, gc.IsNil)
}

func (s *notifyWorkerSuite) TestWait(c *gc.C) {
	done := make(chan error)
	go func() {
		done <- s.worker.Wait()
	}()
	select {
	case err := <-done:
		c.Errorf("Wait() didn't wait until we stopped it. err: %v", err)
	case <-time.After(shortWait):
	}
	s.worker.Kill()
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(longWait):
		c.Errorf("Wait() failed to return after we stopped.")
	}
}

func (s *notifyWorkerSuite) TestCallSetUpAndTearDown(c *gc.C) {
	// After calling NewNotifyWorker, we should have called setup
	s.actor.CheckActions(c, "setup")
	// If we kill the worker, it should notice, and call teardown
	s.worker.Kill()
	err := WaitShort(c, s.worker)
	c.Check(err, gc.IsNil)
	s.actor.CheckActions(c, "setup", "teardown")
	c.Check(s.actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestChangesTriggerHandler(c *gc.C) {
	s.actor.CheckActions(c, "setup")
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler")
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.watcher.TriggerChange(c)
	WaitForHandled(c, s.actor.handled)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler")
	c.Assert(s.worker.Stop(), gc.IsNil)
	s.actor.CheckActions(c, "setup", "handler", "handler", "handler", "teardown")
}

func (s *notifyWorkerSuite) TestSetUpFailureStopsWithTearDown(c *gc.C) {
	// Stop the worker and SetUp again, this time with an error
	s.stopWorker(c)
	actor := &ActionsHandler{
		actions:    nil,
		handled:    make(chan struct{}),
		setupError: fmt.Errorf("my special error"),
		watcher: &TestWatcher{
			out: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my special error")
	actor.CheckActions(c, "setup", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}

func (s *notifyWorkerSuite) TestSetupNilWatcherStopsWithTearDown(c *gc.C) {
	s.stopWorker(c)
	actor := &ActionsHandler{
		watcher: nil,
	}
	w := worker.NewNotifyWorker(actor)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "SetUp returned a nil Watcher")
	actor.CheckActions(c, "setup", "teardown")
}

func (s *notifyWorkerSuite) TestWatcherStopFailurePropagates(c *gc.C) {
	s.actor.watcher.SetStopError(fmt.Errorf("error while stopping watcher"))
	s.worker.Kill()
	c.Assert(s.worker.Wait(), gc.ErrorMatches, "error while stopping watcher")
	// We've already stopped the worker, don't let teardown notice the
	// worker is in an error state
	s.worker = nil
}

func (s *notifyWorkerSuite) TestHandleErrorStopsWorkerAndWatcher(c *gc.C) {
	s.stopWorker(c)
	actor := &ActionsHandler{
		actions:      nil,
		handled:      make(chan struct{}),
		handlerError: fmt.Errorf("my handling error"),
		watcher: &TestWatcher{
			out: make(chan struct{}),
		},
	}
	w := worker.NewNotifyWorker(actor)
	actor.watcher.TriggerChange(c)
	WaitForHandled(c, actor.handled)
	err := WaitShort(c, w)
	c.Check(err, gc.ErrorMatches, "my handling error")
	actor.CheckActions(c, "setup", "handler", "teardown")
	c.Check(actor.watcher.stopped, jc.IsTrue)
}
