// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type dummyLogSinkState struct {
	model *coremodel.ModelInfo
}

func (d *dummyLogSinkState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	if d.model != nil {
		return *d.model, nil
	}
	return coremodel.ModelInfo{}, modelerrors.NotFound
}

type logSinkServiceSuite struct {
	testing.IsolationSuite

	state *dummyLogSinkState
}

var _ = gc.Suite(&logSinkServiceSuite{})

func (s *logSinkServiceSuite) SetUpTest(c *gc.C) {
	s.state = &dummyLogSinkState{}
}

func (s *logSinkServiceSuite) TestModel(c *gc.C) {
	svc := NewLogSinkService(s.state)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.ModelInfo{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	s.state.model = &model

	got, err := svc.Model(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(got, gc.Equals, model)
}
