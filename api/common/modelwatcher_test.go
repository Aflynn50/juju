// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type modelwatcherTests struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&modelwatcherTests{})

func (s *modelwatcherTests) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *modelwatcherTests) TestModelConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	attrs := testing.FakeConfig()
	attrs["logging-config"] = "<root>=INFO"

	facade := apimocks.NewMockFacadeCaller(ctrl)
	result := params.ModelConfigResult{
		Config: params.ModelConfig(attrs),
	}
	facade.EXPECT().FacadeCall("ModelConfig", nil, gomock.Any()).SetArg(2, result).Return(nil)

	client := common.NewModelWatcher(facade)
	cfg, err := client.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Attrs(cfg.AllAttrs()), gc.DeepEquals, attrs)
}

func (s *modelwatcherTests) TestWatchForModelConfigChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	facade := apimocks.NewMockFacadeCaller(ctrl)
	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("NotifyWatcher").Return(666)
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Next", nil, gomock.Any()).Return(nil).AnyTimes()
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Stop", nil, gomock.Any()).Return(nil).AnyTimes()

	result := params.NotifyWatchResult{}
	facade.EXPECT().FacadeCall("WatchForModelConfigChanges", nil, gomock.Any()).SetArg(2, result).Return(nil)
	facade.EXPECT().RawAPICaller().Return(caller)

	client := common.NewModelWatcher(facade)
	w, err := client.WatchForModelConfigChanges()
	c.Assert(err, jc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-w.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}
