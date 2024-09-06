// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"golang.org/x/net/context"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretManagerAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(stdCtx context.Context, ctx facade.ModelContext) (*SecretsManagerAPI, error) {
	if !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	serviceFactory := ctx.ServiceFactory()
	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backendService := serviceFactory.SecretBackend()
	modelInfoService := serviceFactory.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendAdminConfigGetter := func(stdCtx context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, modelInfo.UUID)
	}
	backendUserSecretConfigGetter := func(
		stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secretservice.SecretAccessor,
	) (*provider.ModelBackendConfigInfo, error) {
		return backendService.BackendConfigInfo(stdCtx, secretbackendservice.BackendConfigParams{
			GrantedSecretsGetter: gsg,
			Accessor:             accessor,
			ModelUUID:            modelInfo.UUID,
			SameController:       true,
		})
	}
	secretService := serviceFactory.Secret(secretBackendAdminConfigGetter, backendUserSecretConfigGetter)

	controllerAPI := common.NewControllerConfigAPI(
		ctx.State(),
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
	)
	remoteClientGetter := func(stdCtx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error) {
		info, err := controllerAPI.ControllerAPIInfoForModels(stdCtx, params.Entities{Entities: []params.Entity{{
			Tag: names.NewModelTag(uri.SourceUUID).String(),
		}}})
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(info.Results) < 1 {
			return nil, errors.Errorf("no controller api for model %q", uri.SourceUUID)
		}
		if err := info.Results[0].Error; err != nil {
			return nil, errors.Trace(err)
		}
		apiInfo := api.Info{
			Addrs:    info.Results[0].Addresses,
			CACert:   info.Results[0].CACert,
			ModelTag: names.NewModelTag(uri.SourceUUID),
		}
		apiInfo.Tag = names.NewUserTag(api.AnonymousUsername)
		conn, err := apicaller.NewExternalControllerConnection(stdCtx, &apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodelsecrets.NewClient(conn), nil
	}

	return &SecretsManagerAPI{
		authTag:              ctx.Auth().GetAuthTag(),
		authorizer:           ctx.Auth(),
		leadershipChecker:    leadershipChecker,
		watcherRegistry:      ctx.WatcherRegistry(),
		secretBackendService: backendService,
		secretService:        secretService,
		secretsTriggers:      secretService,
		secretsConsumer:      secretService,
		clock:                clock.WallClock,
		controllerUUID:       ctx.ControllerUUID(),
		modelUUID:            ctx.State().ModelUUID(),
		remoteClientGetter:   remoteClientGetter,
		crossModelState:      ctx.State().RemoteEntities(),
		logger:               ctx.Logger().Child("secretsmanager", corelogger.SECRETS),
	}, nil
}
