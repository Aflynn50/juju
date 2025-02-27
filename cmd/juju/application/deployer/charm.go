// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v12"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	applicationapi "github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	app "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/storage"
)

type deployCharm struct {
	applicationName  string
	attachStorage    []string
	bindings         map[string]string
	configOptions    DeployConfigFlag
	constraints      constraints.Value
	dryRun           bool
	modelConstraints constraints.Value
	devices          map[string]devices.Constraints
	deployResources  DeployResourcesFunc
	force            bool
	id               application.CharmID
	flagSet          *gnuflag.FlagSet
	model            ModelCommand
	numUnits         int
	placement        []*instance.Placement
	placementSpec    string
	resources        map[string]string
	baseFlag         corebase.Base
	storage          map[string]storage.Constraints
	trust            bool

	validateCharmBaseWithName             func(base corebase.Base, name string, imageStream string) error
	validateResourcesNeededForLocalDeploy func(charmMeta *charm.Meta) error
}

// deploy is the business logic of deploying a charm after
// it's been prepared.
func (d *deployCharm) deploy(
	ctx *cmd.Context,
	deployAPI DeployerAPI,
) (rErr error) {
	id := d.id
	charmInfo, err := deployAPI.CharmInfo(id.URL.String())
	if err != nil {
		return err
	}
	checkPodspec(charmInfo.Charm(), ctx)

	// storage cannot be added to a container.
	if len(d.storage) > 0 || len(d.attachStorage) > 0 {
		for _, placement := range d.placement {
			if t, err := instance.ParseContainerType(placement.Scope); err == nil {
				return errors.NotSupportedf("adding storage to %s container", string(t))
			}
		}
	}

	numUnits := d.numUnits
	if charmInfo.Meta.Subordinate {
		if !constraints.IsEmpty(&d.constraints) {
			return errors.New("cannot use --constraints with subordinate application")
		}
		if numUnits == 1 && d.placementSpec == "" {
			numUnits = 0
		} else {
			return errors.New("cannot use --num-units or --to with subordinate application")
		}
	}
	applicationName := d.applicationName
	if applicationName == "" {
		applicationName = charmInfo.Meta.Name
	}

	// Process the --config args.
	appConfig, configYAML, err := utils.ProcessConfig(ctx, d.model.Filesystem(), d.configOptions, &d.trust)
	if err != nil {
		return errors.Trace(err)
	}
	// At deploy time, there's no need to include "trust=false" as missing is the same thing.
	if !d.trust {
		delete(appConfig, app.TrustConfigOptionName)
	}

	if id.URL != nil && id.URL.Schema != "local" && len(charmInfo.Meta.Terms) > 0 {
		ctx.Infof("Deployment under prior agreement to terms: %s",
			strings.Join(charmInfo.Meta.Terms, " "))
	}

	ids, err := d.deployResources(
		applicationName,
		resources.CharmID{
			URL:    id.URL,
			Origin: id.Origin,
		},
		d.resources,
		charmInfo.Meta.Resources,
		deployAPI,
		d.model.Filesystem(),
	)
	if err != nil {
		return errors.Trace(err)
	}

	if len(appConfig) == 0 {
		appConfig = nil
	}

	ctx.Infof(d.formatDeployingText())
	args := applicationapi.DeployArgs{
		CharmID:          id,
		CharmOrigin:      id.Origin,
		Cons:             d.constraints,
		ApplicationName:  applicationName,
		NumUnits:         numUnits,
		ConfigYAML:       configYAML,
		Config:           appConfig,
		Placement:        d.placement,
		Storage:          d.storage,
		Devices:          d.devices,
		AttachStorage:    d.attachStorage,
		Resources:        ids,
		EndpointBindings: d.bindings,
		Force:            d.force,
	}

	err = deployAPI.Deploy(args)
	if err == nil {
		return nil
	}

	if errors.IsAlreadyExists(err) {
		// Would be nice to be able to access the app name here
		return errors.Wrapf(err, errors.Errorf(`
deploy application using an alias name:
    juju deploy <application> <alias>
or use remove-application to remove the existing one and try again.`,
		), err.Error())
	}
	return errors.Trace(err)
}

var (
	// BundleOnlyFlags represents what flags are used for bundles only.
	BundleOnlyFlags = []string{
		"overlay", "map-machines",
	}
)

func (d *deployCharm) validateCharmFlags() error {
	if flags := utils.GetFlags(d.flagSet, BundleOnlyFlags); len(flags) > 0 {
		return errors.Errorf("options provided but not supported when deploying a charm: %s", strings.Join(flags, ", "))
	}
	return nil
}

func (d *deployCharm) formatDeployingText() string {
	curl := d.id.URL
	name := d.applicationName
	if name == "" {
		name = curl.Name
	}
	origin := d.id.Origin
	channel := origin.CharmChannel().String()
	if channel != "" {
		channel = fmt.Sprintf(" in channel %s", channel)
	}

	return fmt.Sprintf("Deploying %q from %s charm %q, revision %d%s on %s",
		name, origin.Source, curl.Name, curl.Revision, channel, origin.Base.String())
}

type predeployedLocalCharm struct {
	deployCharm
	userCharmURL *charm.URL
}

// String returns a string description of the deployer.
func (d *predeployedLocalCharm) String() string {
	str := fmt.Sprintf("deploy predeployed local charm: %s", d.userCharmURL.String())
	origin := d.id.Origin
	if isEmptyOrigin(origin, commoncharm.OriginLocal) {
		return str
	}
	var channel string
	if ch := origin.CharmChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s", str, channel)
}

// PrepareAndDeploy finishes preparing to deploy a predeployed local charm,
// then deploys it.
func (d *predeployedLocalCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ Resolver) error {
	userCharmURL := d.userCharmURL
	ctx.Verbosef("Preparing to deploy local charm %q again", userCharmURL.Name)
	if d.dryRun {
		ctx.Infof("ignoring dry-run flag for local charms")
	}

	modelCfg, err := getModelConfig(deployAPI)
	if err != nil {
		return errors.Trace(err)
	}

	// Avoid deploying charm if it's not valid for the model.
	base, err := corebase.GetBaseFromSeries(d.userCharmURL.Series)
	if err != nil {
		return errors.Trace(err)
	}
	if err := d.validateCharmBaseWithName(base, userCharmURL.Name, modelCfg.ImageStream()); err != nil {
		return errors.Trace(err)
	}

	if err := d.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	charmInfo, err := deployAPI.CharmInfo(d.userCharmURL.String())
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof(formatLocatedText(d.userCharmURL, commoncharm.Origin{}))
	checkPodspec(charmInfo.Charm(), ctx)

	if err := d.validateResourcesNeededForLocalDeploy(charmInfo.Meta); err != nil {
		return errors.Trace(err)
	}

	platform := utils.MakePlatform(d.constraints, base, d.modelConstraints)
	origin, err := utils.MakeOrigin(charm.Local, userCharmURL.Revision, charm.Channel{}, platform)
	if err != nil {
		return errors.Trace(err)
	}

	d.id = application.CharmID{
		URL:    d.userCharmURL,
		Origin: origin,
	}
	return d.deploy(ctx, deployAPI)
}

type localCharm struct {
	deployCharm
	curl *charm.URL
	base corebase.Base
	ch   charm.Charm
}

// String returns a string description of the deployer.
func (l *localCharm) String() string {
	return fmt.Sprintf("deploy local charm: %s", l.curl.String())
}

// PrepareAndDeploy finishes preparing to deploy a local charm,
// then deploys it.
func (l *localCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, _ Resolver) error {
	ctx.Verbosef("Preparing to deploy local charm: %q ", l.curl.Name)
	if l.dryRun {
		ctx.Infof("ignoring dry-run flag for local charms")
	}
	if err := l.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	curl, err := deployAPI.AddLocalCharm(l.curl, l.ch, l.force)
	if err != nil {
		return errors.Trace(err)
	}

	platform := utils.MakePlatform(l.constraints, l.base, l.modelConstraints)
	// Local charms don't need a channel.
	origin, err := utils.MakeOrigin(charm.Local, curl.Revision, charm.Channel{}, platform)
	if err != nil {
		return errors.Trace(err)
	}

	ctx.Infof(formatLocatedText(curl, origin))
	l.id = application.CharmID{
		URL:    curl,
		Origin: origin,
	}
	return l.deploy(ctx, deployAPI)
}

type repositoryCharm struct {
	deployCharm
	userRequestedURL               *charm.URL
	clock                          jujuclock.Clock
	uploadExistingPendingResources UploadExistingPendingResourcesFunc
}

// String returns a string description of the deployer.
func (c *repositoryCharm) String() string {
	str := fmt.Sprintf("deploy charm: %s", c.userRequestedURL.String())
	origin := c.id.Origin
	if isEmptyOrigin(origin, commoncharm.OriginCharmHub) {
		return str
	}
	var revision string
	if origin.Revision != nil && *origin.Revision != -1 {
		revision = fmt.Sprintf(" with revision %d", *origin.Revision)
	}
	var channel string
	if ch := origin.CharmChannel().String(); ch != "" {
		if revision != "" {
			channel = fmt.Sprintf(" will refresh from channel %s", ch)
		} else {
			channel = fmt.Sprintf(" from channel %s", ch)
		}
	}
	return fmt.Sprintf("%s%s%s", str, revision, channel)
}

// PrepareAndDeploy finishes preparing to deploy a repository charm,
// then deploys it.
func (c *repositoryCharm) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver) error {
	if deployAPI.BestFacadeVersion("Application") < 19 {
		return c.compatibilityPrepareAndDeploy(ctx, deployAPI, resolver)
	}
	var base *corebase.Base
	if !c.baseFlag.Empty() {
		base = &c.baseFlag
	}
	if err := c.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	var channel *string
	if c.id.Origin.CharmChannel().String() != "" {
		str := c.id.Origin.CharmChannel().String()
		channel = &str
	}

	// Process the --config args.
	appNameForConfig := c.userRequestedURL.Name
	if c.applicationName != "" {
		appNameForConfig = c.applicationName
	}

	configYAML, err := utils.CombinedConfig(ctx, c.model.Filesystem(), c.configOptions, appNameForConfig)
	if err != nil {
		return errors.Trace(err)
	}

	charmName := c.userRequestedURL.Name
	info, localPendingResources, errs := deployAPI.DeployFromRepository(application.DeployFromRepositoryArg{
		CharmName:        charmName,
		ApplicationName:  c.applicationName,
		AttachStorage:    c.attachStorage,
		Base:             base,
		Channel:          channel,
		ConfigYAML:       configYAML,
		Cons:             c.constraints,
		Devices:          c.devices,
		DryRun:           c.dryRun,
		EndpointBindings: c.bindings,
		Force:            c.force,
		NumUnits:         &c.numUnits,
		Placement:        c.placement,
		Revision:         c.id.Origin.Revision,
		Resources:        c.resources,
		Storage:          c.storage,
		Trust:            c.trust,
	})

	for _, err := range errs {
		ctx.Errorf(err.Error())
	}
	if len(errs) != 0 {
		return errors.Errorf("failed to deploy charm %q", charmName)
	}

	// No localPendingResources should exist if a dry-run.
	uploadErr := c.uploadExistingPendingResources(info.Name, localPendingResources, deployAPI,
		c.model.Filesystem())
	if uploadErr != nil {
		ctx.Errorf("Unable to upload resources for %v, consider using --attach-resource. \n %v",
			info.Name, uploadErr)
	}

	ctx.Infof(formatDeployedText(c.dryRun, charmName, info))
	return nil
}

func formatDeployedText(dryRun bool, charmName string, info application.DeployInfo) string {
	if dryRun {
		return fmt.Sprintf("%q from charm-hub charm %q, revision %d in channel %s on %s would be deployed",
			info.Name, charmName, info.Revision, info.Channel, info.Base.String())
	}
	return fmt.Sprintf("Deployed %q from charm-hub charm %q, revision %d in channel %s on %s",
		info.Name, charmName, info.Revision, info.Channel, info.Base.String())
}

// compatibilityPrepareAndDeploy deploys repository charms to controllers
// which do not have application facade 19 or greater.
func (c *repositoryCharm) compatibilityPrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver) error {
	userRequestedURL := c.userRequestedURL
	location := "charmhub"

	ctx.Verbosef("Preparing to deploy %q from the %s", userRequestedURL.Name, location)

	modelCfg, err := getModelConfig(deployAPI)
	if err != nil {
		return errors.Trace(err)
	}

	// Charm or bundle has been supplied as a URL so we resolve and
	// deploy using the store but pass in the origin command line
	// argument so users can target a specific origin.
	origin := c.id.Origin
	var usingDefaultBase bool
	if defaultBase, ok := modelCfg.DefaultBase(); ok && origin.Base.Channel.Empty() {
		base, err := corebase.ParseBaseFromString(defaultBase)
		if err != nil {
			return errors.Trace(err)
		}
		origin.Base = base
		usingDefaultBase = true
	}
	storeCharmOrBundleURL, origin, supportedBases, err := resolver.ResolveCharm(userRequestedURL, origin, false) // no --switch possible.
	if charm.IsUnsupportedSeriesError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usingDefaultBase {
			msg += " Used the default-base."
		}
		return errors.Errorf(msg)
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := c.validateCharmFlags(); err != nil {
		return errors.Trace(err)
	}

	imageStream := modelCfg.ImageStream()
	workloadBases, err := SupportedJujuBases(c.clock.Now(), c.baseFlag, imageStream)
	if err != nil {
		return errors.Trace(err)
	}

	baseSelector, err := corecharm.ConfigureBaseSelector(corecharm.SelectorConfig{
		Config:              modelCfg,
		Force:               c.force,
		Logger:              logger,
		RequestedBase:       c.baseFlag,
		SupportedCharmBases: supportedBases,
		WorkloadBases:       workloadBases,
		UsingImageID:        (c.constraints.HasImageID() || c.modelConstraints.HasImageID()),
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Get the base to use.
	base, err := baseSelector.CharmBase()
	logger.Tracef("Using base %q from %v to deploy %v", base, supportedBases, userRequestedURL)
	if corecharm.IsUnsupportedBaseError(err) {
		msg := fmt.Sprintf("%v. Use --force to deploy the charm anyway.", err)
		if usingDefaultBase {
			msg += " Used the default-base."
		}
		return errors.Errorf(msg)
	}
	if validationErr := charmValidationError(storeCharmOrBundleURL.Name, errors.Trace(err)); validationErr != nil {
		return errors.Trace(validationErr)
	}

	// Ensure we save the origin.
	origin = origin.WithBase(&base)

	// In-order for the url to represent the following updates to the origin
	// and machine, we need to ensure that the series is actually correct as
	// well in the url.
	curl := storeCharmOrBundleURL
	if charm.CharmHub.Matches(storeCharmOrBundleURL.Schema) {
		series, err := corebase.GetSeriesFromBase(origin.Base)
		if err != nil {
			return errors.Trace(err)
		}
		curl = storeCharmOrBundleURL.WithSeries(series)
	}

	if c.dryRun {
		name := c.applicationName
		if name == "" {
			name = curl.Name
		}
		channel := origin.CharmChannel().String()
		if channel != "" {
			channel = fmt.Sprintf(" in channel %s", channel)
		}

		ctx.Infof(fmt.Sprintf("%q from %s charm %q, revision %d%s on %s would be deployed",
			name, origin.Source, curl.Name, curl.Revision, channel, origin.Base.DisplayString()))
		return nil
	}

	// Store the charm in the controller
	csOrigin, err := deployAPI.AddCharm(curl, origin, c.force)
	if err != nil {
		if termErr, ok := errors.Cause(err).(*common.TermsRequiredError); ok {
			return errors.Trace(termErr.UserErr())
		}
		if origin.Source != commoncharm.OriginLocal && origin.Source != commoncharm.OriginCharmHub {
			// The following error raise is an assumption. If we have an alternative source at some point,
			// then this error will have to change. The proper way to do this is to have a separate
			// error type that's raised in AddCharmWithAuthorizationFromURL above for unsupported schema.
			return errors.Annotatef(err, "schema %q not supported", origin.Source)
		}
		return errors.Annotatef(err, "storing charm %q", curl.Name)
	}
	ctx.Infof(formatLocatedText(curl, csOrigin))

	// If the original base was empty, so we couldn't validate the original
	// charm base, but the charm url wasn't nil, we can check and validate
	// what that one says.
	//
	// Note: it's interesting that the charm url and the series can diverge and
	// tell different things when deploying a charm and in sake of understanding
	// what we deploy, we should converge the two so that both report identical
	// values.
	if curl != nil && base.Empty() {
		b, err := corebase.GetBaseFromSeries(curl.Series)
		if err != nil {
			return errors.Trace(err)
		}
		if err := c.validateCharmBaseWithName(b, curl.Name, imageStream); err != nil {
			return errors.Trace(err)
		}
	}

	c.id = application.CharmID{
		URL:    curl,
		Origin: csOrigin,
	}
	return c.deploy(ctx, deployAPI)
}

func isEmptyOrigin(origin commoncharm.Origin, source commoncharm.OriginSource) bool {
	other := commoncharm.Origin{}
	if origin == other {
		return true
	}
	other.Source = source
	return origin == other
}

func formatLocatedText(curl *charm.URL, origin commoncharm.Origin) string {
	repository := origin.Source
	if repository == "" || repository == commoncharm.OriginLocal {
		return fmt.Sprintf("Located local charm %q, revision %d", curl.Name, curl.Revision)
	}
	var next string
	if curl.Revision != -1 {
		next = fmt.Sprintf(", revision %d", curl.Revision)
	} else if str := origin.CharmChannel().String(); str != "" {
		next = fmt.Sprintf(", channel %s", str)
	}
	return fmt.Sprintf("Located charm %q in %s%s", curl.Name, repository, next)
}
