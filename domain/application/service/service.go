// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	internalcharm "github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// State represents a type for interacting with the underlying state.
type State interface {
	ApplicationState
	CharmState
	StorageState
	UnitState
}

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")
)

// Service provides the API for working with applications.
type Service struct {
	st            State
	leaderEnsurer leadership.Ensurer
	logger        logger.Logger
	clock         clock.Clock

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	charmStore            CharmStore
	statusHistory         StatusHistory
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	return &Service{
		st:                    st,
		leaderEnsurer:         leaderEnsurer,
		logger:                logger,
		clock:                 clock,
		storageRegistryGetter: storageRegistryGetter,
		charmStore:            charmStore,
		statusHistory:         statusHistory,
	}
}

// AgentVersionGetter is responsible for retrieving the target
// agent version for the current model.
type AgentVersionGetter interface {
	// GetTargetAgentVersion returns the agent version
	// for the current model.
	GetTargetAgentVersion(context.Context) (version.Number, error)
}

// Provider defines the interface for interacting with the underlying model
// provider.
type Provider interface {
	environs.ConstraintsChecker
}

// SupportedFeatureProvider defines the interface for interacting with the
// a model provider that satisfies the SupportedFeatureEnumerator interface.
type SupportedFeatureProvider interface {
	environs.SupportedFeatureEnumerator
}

// ProviderService defines a service for interacting with the underlying
// model state.
type ProviderService struct {
	*Service

	modelID            coremodel.UUID
	agentVersionGetter AgentVersionGetter
	provider           providertracker.ProviderGetter[Provider]
	// This provider is separated from [provider] because the
	// [SupportedFeatureProvider] interface is only satisfied by the
	// k8s provider.
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider]
}

// NewProviderService returns a new Service for interacting with a models state.
func NewProviderService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelID coremodel.UUID,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider],
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: NewService(
			st,
			leaderEnsurer,
			storageRegistryGetter,
			charmStore,
			statusHistory,
			clock,
			logger,
		),
		modelID:                  modelID,
		agentVersionGetter:       agentVersionGetter,
		provider:                 provider,
		supportedFeatureProvider: supportedFeatureProvider,
	}
}

// GetSupportedFeatures returns the set of features that the model makes
// available for charms to use.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (s *ProviderService) GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error) {
	agentVersion, err := s.agentVersionGetter.GetTargetAgentVersion(ctx)
	if err != nil {
		return assumes.FeatureSet{}, err
	}

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})

	supportedFeatureProvider, err := s.supportedFeatureProvider(ctx)
	if errors.Is(err, errors.NotSupported) {
		return fs, nil
	} else if err != nil {
		return fs, err
	}

	envFs, err := supportedFeatureProvider.SupportedFeatures()
	if err != nil {
		return fs, fmt.Errorf("enumerating features supported by environment: %w", err)
	}

	fs.Merge(envFs)

	return fs, nil
}

// SetApplicationConstraints sets the application constraints for the
// specified application ID.
// This method overwrites the full constraints on every call.
// If invalid constraints are provided (e.g. invalid container type or
// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
// error is returned.
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *ProviderService) SetApplicationConstraints(ctx context.Context, appID coreapplication.ID, cons coreconstraints.Value) error {
	if err := appID.Validate(); err != nil {
		return internalerrors.Errorf("application ID: %w", err)
	}
	if err := s.validateConstraints(ctx, cons); err != nil {
		return err
	}

	return s.st.SetApplicationConstraints(ctx, appID, constraints.DecodeConstraints(cons))
}

func (s *ProviderService) constraintsValidator(ctx context.Context) (coreconstraints.Validator, error) {
	provider, err := s.provider(ctx)
	if errors.Is(err, errors.NotSupported) {
		// Not validating constraints, as the provider doesn't support it.
		return nil, nil
	} else if err != nil {
		return nil, internalerrors.Capture(err)
	}

	validator, err := provider.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(ctx))
	if errors.Is(err, errors.NotImplemented) {
		return nil, nil
	} else if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return validator, nil
}

func (s *ProviderService) validateConstraints(ctx context.Context, cons coreconstraints.Value) error {
	validator, err := s.constraintsValidator(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}
	if validator == nil {
		return nil
	}

	unsupported, err := validator.Validate(cons)
	if len(unsupported) > 0 {
		s.logger.Warningf(ctx,
			"unsupported constraints: %v", strings.Join(unsupported, ","))
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	return nil
}

// WatchableService provides the API for working with applications and the
// ability to create watchers.
type WatchableService struct {
	*ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	st State,
	leaderEnsurer leadership.Ensurer,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelID coremodel.UUID,
	watcherFactory WatcherFactory,
	agentVersionGetter AgentVersionGetter,
	provider providertracker.ProviderGetter[Provider],
	supportedFeatureProvider providertracker.ProviderGetter[SupportedFeatureProvider],
	charmStore CharmStore,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		ProviderService: NewProviderService(
			st,
			leaderEnsurer,
			storageRegistryGetter,
			modelID,
			agentVersionGetter,
			provider,
			supportedFeatureProvider,
			charmStore,
			statusHistory,
			clock,
			logger,
		),
		watcherFactory: watcherFactory,
	}
}

// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
func (s *WatchableService) WatchApplicationUnitLife(appName string) (watcher.StringsWatcher, error) {
	lifeGetter := func(ctx context.Context, db database.TxnRunner, ids []string) (map[string]life.Life, error) {
		unitUUIDs, err := transform.SliceOrErr(ids, coreunit.ParseID)
		if err != nil {
			return nil, err
		}
		lives, err := s.st.GetApplicationUnitLife(ctx, appName, unitUUIDs...)
		if err != nil {
			return nil, err
		}
		result := make(map[string]life.Life, len(lives))
		for unitUUID, life := range lives {
			result[unitUUID.String()] = life
		}
		return result, nil
	}
	lifeMapper := domain.LifeStringsWatcherMapperFunc(s.logger, lifeGetter)

	table, query := s.st.InitialWatchStatementUnitLife(appName)
	return s.watcherFactory.NewNamespaceMapperWatcher(table, changestream.All, query, lifeMapper)
}

// WatchApplicationScale returns a watcher that observes changes to an application's scale.
func (s *WatchableService) WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	scaleState, err := s.st.GetApplicationScaleState(ctx, appID)
	if err != nil {
		return nil, internalerrors.Errorf("getting scaling state for %q: %w", appName, err)
	}
	currentScale := scaleState.Scale

	mask := changestream.Changed
	mapper := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		newScaleState, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		newScale := newScaleState.Scale
		// Only dispatch if the scale has changed.
		if newScale != currentScale {
			currentScale = newScale
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewValueMapperWatcher("application_scale", appID.String(), mask, mapper)
}

// WatchApplicationsWithPendingCharms returns a watcher that observes changes to
// applications that have pending charms.
func (s *WatchableService) WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error) {
	table, query := s.st.InitialWatchStatementApplicationsWithPendingCharms()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		table,
		changestream.Changed,
		query,
		func(ctx context.Context, _ database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			return s.watchApplicationsWithPendingCharmsMapper(ctx, changes)
		},
	)
}

// watchApplicationsWithPendingCharmsMapper removes any applications that do not
// have pending charms.
func (s *WatchableService) watchApplicationsWithPendingCharmsMapper(ctx context.Context, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
	// Preserve the ordering of the changes, as this is a strings watcher
	// and we want to return the changes in the order they were received.

	appChanges := make(map[coreapplication.ID][]indexedChanged)
	uuids := make([]coreapplication.ID, 0)
	for i, change := range changes {
		appID, err := coreapplication.ParseID(change.Changed())
		if err != nil {
			return nil, err
		}

		if _, ok := appChanges[appID]; !ok {
			uuids = append(uuids, appID)
		}

		appChanges[appID] = append(appChanges[appID], indexedChanged{
			change: change,
			index:  i,
		})
	}

	// Get all the applications with pending charms using the uuids.
	apps, err := s.GetApplicationsWithPendingCharmsFromUUIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}

	// If any applications have no pending charms, then return no changes.
	if len(apps) == 0 {
		return nil, nil
	}

	// Grab all the changes for the applications with pending charms,
	// ensuring they're indexed so we can sort them later.
	var indexed []indexedChanged
	for _, appID := range apps {
		events, ok := appChanges[appID]
		if !ok {
			s.logger.Errorf(context.TODO(), "application %q has pending charms but no change events", appID)
			continue
		}

		indexed = append(indexed, events...)
	}

	// Sort the index so they're preserved
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].index < indexed[j].index
	})

	// Grab the changes in the order they were received.
	var results []changestream.ChangeEvent
	for _, result := range indexed {
		results = append(results, result.change)
	}

	return results, nil
}

type indexedChanged struct {
	change changestream.ChangeEvent
	index  int
}

// WatchApplication watches for changes to the specified application in the
// application table.
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplication(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, internalerrors.Errorf("getting ID of application %s: %w", name, err)
	}
	return s.watcherFactory.NewValueWatcher(
		"application",
		uuid.String(),
		changestream.All,
	)
}

// WatchApplicationConfig watches for changes to the specified application's
// config.
// This notifies on any changes to the application's config, which is driven
// of the application config hash. It is up to the caller to determine if the
// config value they're interested in has changed.
//
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplicationConfig(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	uuid, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, internalerrors.Errorf("getting ID of application %s: %w", name, err)
	}

	return s.watcherFactory.NewValueWatcher("application_config_hash", uuid.String(), changestream.All)
}

// WatchApplicationConfigHash watches for changes to the specified application's
// config hash.
// This notifies on any changes to the application's config hash, which is
// driven of the application config hash. It is up to the caller to determine
// if the config value they're interested in has changed. This watcher is
// the backing for the uniter's remote state. We should be attempting to
// remove this in the future.
//
// If the application does not exist an error satisfying
// [applicationerrors.NotFound] will be returned.
func (s *WatchableService) WatchApplicationConfigHash(ctx context.Context, name string) (watcher.StringsWatcher, error) {
	appID, err := s.GetApplicationIDByName(ctx, name)
	if err != nil {
		return nil, internalerrors.Errorf("getting ID of application %s: %w", name, err)
	}

	// sha256 is the current config hash for the application. This will
	// be filled in by the initial query. If it's empty after the initial
	// query, then a new config hash will be generated on the first change.
	var sha256 string

	table, query := s.st.InitialWatchStatementApplicationConfigHash(name)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		table,
		changestream.All,
		func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
			initialResults, err := query(ctx, txn)
			if err != nil {
				return nil, errors.Trace(err)
			}

			if num := len(initialResults); num > 1 {
				return nil, internalerrors.Errorf("too many config hashes for application %q", name)
			} else if num == 1 {
				sha256 = initialResults[0]
			}

			return initialResults, nil
		},
		func(ctx context.Context, _ database.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
			// If there are no changes, return no changes.
			if len(changes) == 0 {
				return nil, nil
			}

			currentSHA256, err := s.st.GetApplicationConfigHash(ctx, appID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// If the hash hasn't changed, return no changes. The first sha256
			// might be empty, so if that's the case the currentSHA256 will not
			// be empty. Either way we'll only return changes if the hash has
			// changed.
			if currentSHA256 == sha256 {
				return nil, nil
			}
			sha256 = currentSHA256

			// There can be only one.
			// Select the last change event, which will be naturally ordered
			// by the grouping of the query (CREATE, UPDATE, DELETE).
			change := changes[len(changes)-1]

			return []changestream.ChangeEvent{
				newMaskedChangeIDEvent(change, sha256),
			}, nil
		},
	)
}

type maskedChangeIDEvent struct {
	changestream.ChangeEvent
	id string
}

func newMaskedChangeIDEvent(change changestream.ChangeEvent, id string) changestream.ChangeEvent {
	return maskedChangeIDEvent{
		ChangeEvent: change,
		id:          id,
	}
}

func (m maskedChangeIDEvent) Changed() string {
	return m.id
}

// isValidApplicationName returns whether name is a valid application name.
func isValidApplicationName(name string) bool {
	return validApplication.MatchString(name)
}

// isValidReferenceName returns whether name is a valid reference name.
// This ensures that the reference name is both a valid application name
// and a valid charm name.
func isValidReferenceName(name string) bool {
	return isValidApplicationName(name) && isValidCharmName(name)
}

// addDefaultStorageDirectives fills in default values, replacing any empty/missing values
// in the specified directives.
func addDefaultStorageDirectives(
	ctx context.Context,
	state State,
	modelType coremodel.ModelType,
	allDirectives map[string]storage.Directive,
	storage map[string]internalcharm.Storage,
) (map[string]storage.Directive, error) {
	defaults, err := state.StorageDefaults(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage defaults")
	}
	return domainstorage.StorageDirectivesWithDefaults(storage, modelType, defaults, allDirectives)
}

func validateStorageDirectives(
	ctx context.Context,
	state State,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	modelType coremodel.ModelType,
	allDirectives map[string]storage.Directive,
	meta *internalcharm.Meta,
) error {
	registry, err := storageRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	validator, err := domainstorage.NewStorageDirectivesValidator(modelType, registry, state)
	if err != nil {
		return errors.Trace(err)
	}
	err = validator.ValidateStorageDirectivesAgainstCharm(ctx, allDirectives, meta)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure all stores have directives specified. Defaults should have
	// been set by this point, if the user didn't specify any.
	for name, charmStorage := range meta.Storage {
		if _, ok := allDirectives[name]; !ok && charmStorage.CountMin > 0 {
			return fmt.Errorf("%w for store %q", applicationerrors.MissingStorageDirective, name)
		}
	}
	return nil
}

func encodeChannelAndPlatform(origin corecharm.Origin) (*application.Channel, application.Platform, error) {
	channel, err := encodeChannel(origin.Channel)
	if err != nil {
		return nil, application.Platform{}, errors.Trace(err)
	}

	platform, err := encodePlatform(origin.Platform)
	if err != nil {
		return nil, application.Platform{}, errors.Trace(err)
	}

	return channel, platform, nil

}

func encodeCharmSource(source corecharm.Source) (charm.CharmSource, error) {
	switch source {
	case corecharm.Local:
		return charm.LocalSource, nil
	case corecharm.CharmHub:
		return charm.CharmHubSource, nil
	default:
		return "", internalerrors.Errorf("unknown source %q, expected local or charmhub: %w", source, applicationerrors.CharmSourceNotValid)
	}
}

func encodeChannel(ch *internalcharm.Channel) (*application.Channel, error) {
	// Empty channels (not nil), with empty strings for track, risk and branch,
	// will be normalized to "stable", so aren't officially empty.
	// We need to handle that case correctly.
	if ch == nil {
		return nil, nil
	}

	// Always ensure to normalize the channel before encoding it, so that
	// all channels saved to the database are in a consistent format.
	normalize := ch.Normalize()

	risk, err := encodeChannelRisk(normalize.Risk)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &application.Channel{
		Track:  normalize.Track,
		Risk:   risk,
		Branch: normalize.Branch,
	}, nil
}

func encodeChannelRisk(risk internalcharm.Risk) (application.ChannelRisk, error) {
	switch risk {
	case internalcharm.Stable:
		return application.RiskStable, nil
	case internalcharm.Candidate:
		return application.RiskCandidate, nil
	case internalcharm.Beta:
		return application.RiskBeta, nil
	case internalcharm.Edge:
		return application.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q, expected stable, candidate, beta or edge", risk)
	}
}

func encodePlatform(platform corecharm.Platform) (application.Platform, error) {
	ostype, err := encodeOSType(platform.OS)
	if err != nil {
		return application.Platform{}, errors.Trace(err)
	}

	return application.Platform{
		Channel:      platform.Channel,
		OSType:       ostype,
		Architecture: encodeArchitecture(platform.Architecture),
	}, nil
}

func encodeOSType(os string) (application.OSType, error) {
	switch ostype.OSTypeForName(os) {
	case ostype.Ubuntu:
		return application.Ubuntu, nil
	default:
		return 0, errors.Errorf("unknown os type %q, expected ubuntu", os)
	}
}

func encodeArchitecture(a string) application.Architecture {
	switch a {
	case arch.AMD64:
		return architecture.AMD64
	case arch.ARM64:
		return architecture.ARM64
	case arch.PPC64EL:
		return architecture.PPC64EL
	case arch.S390X:
		return architecture.S390X
	case arch.RISCV64:
		return architecture.RISCV64
	default:
		return architecture.Unknown
	}
}

func ptr[T any](v T) *T {
	return &v
}
