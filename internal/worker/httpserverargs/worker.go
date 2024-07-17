// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/state"
)

type workerConfig struct {
	statePool               *state.StatePool
	controllerConfigService ControllerConfigService
	accessService           AccessService
	bakeryConfigService     BakeryConfigService
	mux                     *apiserverhttp.Mux
	clock                   clock.Clock
	newStateAuthenticatorFn NewStateAuthenticatorFunc
}

func (w workerConfig) Validate() error {
	if w.statePool == nil {
		return errors.NotValidf("empty statePool")
	}
	if w.controllerConfigService == nil {
		return errors.NotValidf("empty controllerConfigService")
	}
	if w.accessService == nil {
		return errors.NotValidf("empty accessService")
	}
	if w.mux == nil {
		return errors.NotValidf("empty mux")
	}
	if w.clock == nil {
		return errors.NotValidf("empty clock")
	}
	if w.newStateAuthenticatorFn == nil {
		return errors.NotValidf("empty newStateAuthenticatorFn")
	}
	return nil
}

type argsWorker struct {
	catacomb        catacomb.Catacomb
	cfg             workerConfig
	authenticator   macaroon.LocalMacaroonAuthenticator
	managedServices *managedServices
}

func newWorker(ctx context.Context, cfg workerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := argsWorker{
		cfg: cfg,
		managedServices: newManagedServices(
			cfg.controllerConfigService,
			cfg.accessService,
			cfg.bakeryConfigService,
		),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.managedServices,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	st, err := w.cfg.statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID := st.ModelUUID()

	authenticator, err := w.cfg.newStateAuthenticatorFn(
		ctx,
		w.cfg.statePool,
		modelUUID,
		w.managedServices,
		w.managedServices,
		w.managedServices,
		w.cfg.mux,
		w.cfg.clock,
		w.catacomb.Dying(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.authenticator = authenticator

	return &w, nil
}

// Kill is part of the worker.Worker interface.
func (w *argsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *argsWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *argsWorker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

// managedServices is a ControllerConfigService and a AccessService that wraps
// the underlying services and cancels the context when the tomb is dying.
// This is because the location of the request is not cancellable, so we need
// the ability to cancel the request when the tomb is dying. This should
// prevent any lockup when the controller is shutting down.
type managedServices struct {
	tomb                    tomb.Tomb
	controllerConfigService ControllerConfigService
	accessService           AccessService
	bakeryConfigService     BakeryConfigService
}

func newManagedServices(
	controllerConfigService ControllerConfigService,
	accessService AccessService,
	bakeryConfigService BakeryConfigService,
) *managedServices {
	w := &managedServices{
		controllerConfigService: controllerConfigService,
		accessService:           accessService,
		bakeryConfigService:     bakeryConfigService,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

// ControllerConfig is part of the ControllerConfigService interface.
func (b *managedServices) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return b.controllerConfigService.ControllerConfig(b.tomb.Context(ctx))
}

// GetUserByAuth is part of the AccessService interface.
func (b *managedServices) GetUserByAuth(ctx context.Context, name string, password auth.Password) (coreuser.User, error) {
	return b.accessService.GetUserByAuth(b.tomb.Context(ctx), name, password)
}

// GetUserByName is part of the AccessService interface.
func (b *managedServices) GetUserByName(ctx context.Context, name string) (coreuser.User, error) {
	return b.accessService.GetUserByName(b.tomb.Context(ctx), name)
}

// ReadUserAccessForTarget returns the access that
// the input user subject has for the input target.
func (b *managedServices) ReadUserAccessForTarget(
	ctx context.Context, subject string, target permission.ID,
) (permission.UserAccess, error) {
	return b.accessService.ReadUserAccessForTarget(b.tomb.Context(ctx), subject, target)
}

// UpdateLastModelLogin updates the last login time for the user with the
// given name.
func (b *managedServices) UpdateLastModelLogin(ctx context.Context, name string, modelUUID coremodel.UUID) error {
	return b.accessService.UpdateLastModelLogin(b.tomb.Context(ctx), name, modelUUID)
}

func (b *managedServices) GetLocalUsersKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.bakeryConfigService.GetLocalUsersKey(ctx)
}

func (b *managedServices) GetLocalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.bakeryConfigService.GetLocalUsersThirdPartyKey(ctx)
}

func (b *managedServices) GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.bakeryConfigService.GetExternalUsersThirdPartyKey(ctx)
}

// Kill is part of the worker.Worker interface.
func (b *managedServices) Kill() {
	b.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (b *managedServices) Wait() error {
	return b.tomb.Wait()
}
