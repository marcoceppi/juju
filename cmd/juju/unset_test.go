// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type UnsetSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	dir string
}

var _ = gc.Suite(&UnsetSuite{})

func (s *UnsetSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)
	s.svc = svc
	s.dir = c.MkDir()
	setupConfigFile(c, s.dir)
}

func (s *UnsetSuite) TestUnsetOptionOneByOneSuccess(c *gc.C) {
	// Set options as preparation.
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	// Unset one by one.
	assertUnsetSuccess(c, s.dir, s.svc, []string{"username"}, charm.Settings{
		"outlook": "hello@world.tld",
	})
	assertUnsetSuccess(c, s.dir, s.svc, []string{"outlook"}, charm.Settings{})
}

func (s *UnsetSuite) TestBlockUnset(c *gc.C) {
	// Set options as preparation.
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	// Block operation
	s.AssertConfigParameterUpdated(c, "block-all-changes", true)

	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(envcmd.Wrap(&UnsetCommand{}), ctx, append([]string{"dummy-service"}, []string{"username"}...))
	c.Check(code, gc.Equals, 1)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock changes.*")
}

func (s *UnsetSuite) TestUnsetOptionMultipleAtOnceSuccess(c *gc.C) {
	// Set options as preparation.
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	// Unset multiple options at once.
	assertUnsetSuccess(c, s.dir, s.svc, []string{"username", "outlook"}, charm.Settings{})
}

func (s *UnsetSuite) TestUnsetOptionFail(c *gc.C) {
	assertUnsetFail(c, s.dir, []string{}, "error: no configuration options specified\n")
	assertUnsetFail(c, s.dir, []string{"invalid"}, "error: unknown option \"invalid\"\n")
	assertUnsetFail(c, s.dir, []string{"username=bar"}, "error: unknown option \"username=bar\"\n")
	assertUnsetFail(c, s.dir, []string{
		"username",
		"outlook",
		"invalid",
	}, "error: unknown option \"invalid\"\n")
}

// assertUnsetSuccess unsets configuration options and checks the expected settings.
func assertUnsetSuccess(c *gc.C, dir string, svc *state.Service, args []string, expect charm.Settings) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(&UnsetCommand{}), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Equals, 0)
	settings, err := svc.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

// assertUnsetFail unsets configuration options and checks the expected error.
func assertUnsetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(&UnsetCommand{}), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}
