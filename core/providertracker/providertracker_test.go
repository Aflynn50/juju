// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

type providerSuite struct {
	testing.IsolationSuite

	provider *MockProvider

	providerFactory *MockProviderFactory
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestProviderRunner(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(s.provider, nil)

	runner := ProviderRunner[Provider](s.providerFactory, "foo")
	v, err := runner(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.DeepEquals, s.provider)
}

func (s *providerSuite) TestProviderRunnerSubsetType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[FooProvider](s.providerFactory, "foo")
	v, err := runner(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.DeepEquals, provider)
}

func (s *providerSuite) TestProviderRunnerIsNotSubsetType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[BarProvider](s.providerFactory, "foo")
	_, err := runner(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(s.provider, nil)

	runner := EphemeralProviderRunnerFromConfig[Provider](s.providerFactory, config)

	var provider Provider
	err := runner(context.Background(), func(ctx context.Context, p Provider) error {
		provider = p
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, gc.DeepEquals, s.provider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigSubsetType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[FooProvider](s.providerFactory, config)

	var provider FooProvider
	err := runner(context.Background(), func(ctx context.Context, p FooProvider) error {
		provider = p
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, gc.DeepEquals, fooProvider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigIsNotSubsetType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[BarProvider](s.providerFactory, config)
	err := runner(context.Background(), func(ctx context.Context, p BarProvider) error {
		c.Fail()
		return nil
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *providerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.provider = NewMockProvider(ctrl)

	s.providerFactory = NewMockProviderFactory(ctrl)

	return ctrl
}

type fooProvider struct {
	Provider
}

func (fooProvider) Hello() string {
	return "Hello"
}

type FooProvider interface {
	Hello() string
}

type BarProvider interface {
	World() string
}
