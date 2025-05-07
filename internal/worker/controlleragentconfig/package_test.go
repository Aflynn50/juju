// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"testing"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
