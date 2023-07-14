// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	time "time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/leaseexpiry"
)

type workerSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	store := NewMockExpiryStore(ctrl)

	validCfg := leaseexpiry.Config{
		Clock:  clock.WallClock,
		Logger: jujujujutesting.CheckLogger{Log: c},
		Store:  store,
	}

	cfg := validCfg
	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.Store = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *workerSuite) TestWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clk := NewMockClock(ctrl)
	timer := NewMockTimer(ctrl)
	store := NewMockExpiryStore(ctrl)

	clk.EXPECT().NewTimer(time.Second).Return(timer)
	store.EXPECT().ExpireLeases(gomock.Any()).Return(nil)

	done := make(chan struct{})

	ch := make(chan time.Time, 1)
	ch <- time.Now()
	timer.EXPECT().Chan().Return(ch).MinTimes(1)
	timer.EXPECT().Reset(time.Second).Do(func(any) {
		defer close(done)
	})
	timer.EXPECT().Stop().Return(true)

	w, err := leaseexpiry.NewWorker(leaseexpiry.Config{
		Clock:  clk,
		Logger: jujujujutesting.CheckLogger{Log: c},
		Store:  store,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(jujujujutesting.ShortWait):
		c.Fatalf("timed out waiting for reset")
	}

	workertest.CleanKill(c, w)
}
