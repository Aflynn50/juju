// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreservices

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ChangeStreamName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewObjectStoreServices = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewObjectStoreServicesGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	getter := map[string]any{
		"changestream": s.dbGetter,
	}

	manifold := Manifold(ManifoldConfig{
		ChangeStreamName:             "changestream",
		Logger:                       s.logger,
		NewWorker:                    NewWorker,
		NewObjectStoreServices:       NewObjectStoreServices,
		NewObjectStoreServicesGetter: NewObjectStoreServicesGetter,
	})
	w, err := manifold.Start(context.Background(), dt.StubGetter(getter))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
}

func (s *manifoldSuite) TestOutputObjectStoreServicesGetter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBGetter:                     s.dbGetter,
		Logger:                       s.logger,
		NewObjectStoreServices:       NewObjectStoreServices,
		NewObjectStoreServicesGetter: NewObjectStoreServicesGetter,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory services.ObjectStoreServicesGetter
	err = manifold.output(w, &factory)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldSuite) TestOutputInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBGetter:                     s.dbGetter,
		Logger:                       s.logger,
		NewObjectStoreServices:       NewObjectStoreServices,
		NewObjectStoreServicesGetter: NewObjectStoreServicesGetter,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory struct{}
	err = manifold.output(w, &factory)
	c.Assert(err, gc.ErrorMatches, `unsupported output type .*`)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ChangeStreamName: "changestream",
		Logger:           s.logger,
		NewWorker: func(Config) (worker.Worker, error) {
			return nil, nil
		},
		NewObjectStoreServices: func(model.UUID, changestream.WatchableDBGetter, logger.Logger) services.ObjectStoreServices {
			return nil
		},
		NewObjectStoreServicesGetter: func(ObjectStoreServicesFn, changestream.WatchableDBGetter, logger.Logger) services.ObjectStoreServicesGetter {
			return nil
		},
	}
}
