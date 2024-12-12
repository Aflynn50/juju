// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"fmt"
	"io"

	"github.com/im7mortal/kmutex"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/state"
)

// ResourceOpenerArgs are common arguments for the 2
// types of ResourceOpeners: for unit and for application.
type ResourceOpenerArgs struct {
	State              *state.State
	ModelConfigService ModelConfigService
	ResourceService    ResourceService
	Store              objectstore.ObjectStore
}

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpener(
	args ResourceOpenerArgs,
	resourceDownloadLock func() ResourceDownloadLock,
	unitName string,
) (opener resource.Opener, err error) {
	unit, err := args.State.Unit(unitName)
	if err != nil {
		return nil, errors.Errorf("loading unit: %w", err)
	}

	applicationName := unit.ApplicationName()
	application, err := args.State.Application(applicationName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	chURLStr := unit.CharmURL()
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", applicationName)
	}

	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &ResourceOpener{
		state:                args.State.Resources(args.Store),
		resourceService:      args.ResourceService,
		modelUUID:            args.State.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:          unit.Tag(),
		charmURL:             charmURL,
		charmOrigin:          *application.CharmOrigin(),
		appName:              applicationName,
		unitName:             unitName,
		resourceDownloadLock: resourceDownloadLock,
	}, nil
}

// NewResourceOpenerForApplication returns a new resource.Opener for the given app.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForApplication(
	args ResourceOpenerArgs,
	applicationName string,
) (opener resource.Opener, err error) {
	application, err := args.State.Application(applicationName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	chURLStr, _ := application.CharmURL()
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", applicationName)
	}

	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &ResourceOpener{
		state:                args.State.Resources(args.Store),
		resourceService:      args.ResourceService,
		modelUUID:            args.State.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:          application.Tag(),
		charmURL:             charmURL,
		charmOrigin:          *application.CharmOrigin(),
		appName:              applicationName,
		unitName:             "",
		resourceDownloadLock: func() ResourceDownloadLock {
			return noopDownloadResourceLocker{}
		},
	}, nil
}

func newClientGetter(charmURL *charm.URL, modelConfigService ModelConfigService) resourceClientGetterFunc {
	var clientGetter resourceClientGetterFunc
	switch {
	case charm.CharmHub.Matches(charmURL.Schema):
		clientGetter = newCharmHubOpener(modelConfigService)
	default:
		// Use the nop opener that performs no store side requests. Instead, it
		// will resort to using the state package only. Anything else will call
		// a not-found error.
		clientGetter = newNopOpener()
	}
	return clientGetter
}

// noopDownloadResourceLocker is a no-op download resource locker.
type noopDownloadResourceLocker struct{}

// Acquire grabs the lock for a given application so long as the
// per-application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (noopDownloadResourceLocker) Acquire(string) {}

// Release releases the lock for the given application.
func (noopDownloadResourceLocker) Release(appName string) {}

type resourceClientGetterFunc func(ctx context.Context) (*ResourceRetryClient, error)

// ResourceOpener is a ResourceOpener for charmhub.
// It will first look in the supplied cache for the
// requested resource.
type ResourceOpener struct {
	modelUUID       string
	state           Resources
	resourceService ResourceService
	retrievedBy     names.Tag
	charmURL        *charm.URL
	charmOrigin     state.CharmOrigin
	appName         string
	unitName        string

	resourceClientGetter resourceClientGetterFunc
	resourceDownloadLock func() ResourceDownloadLock
}

// OpenResource implements server.ResourceOpener.
func (ro ResourceOpener) OpenResource(ctx context.Context, name string) (opener resource.Opened, err error) {
	appKey := fmt.Sprintf("%s:%s", ro.modelUUID, ro.appName)
	lock := ro.resourceDownloadLock()
	lock.Acquire(appKey)

	done := func() {
		lock.Release(appKey)
	}
	res, reader, err := ro.getResource(ctx, name, done)
	if err != nil {
		return resource.Opened{}, errors.Capture(err)
	}

	opened := resource.Opened{
		Size:        res.Size,
		Fingerprint: res.Fingerprint,
		ReadCloser:  reader,
	}
	return opened, nil
}

var resourceMutex = kmutex.New()

// GetResource returns a reader for the resource's data.
//
// If the resource is already stored on to the controller then the resource is
// read from there. Otherwise, it is downloaded from charmhub and saved on the
// controller. If the resource name is not known then [errors.NotFound] is
// returned.
func (ro ResourceOpener) getResource(ctx context.Context, resName string, done func()) (_ resource.Resource, rdr io.ReadCloser, err error) {
	defer func() {
		if err == nil {
			rdr = &resourceAccess{
				ReadCloser: rdr,
				done:       done,
			}
		} else {
			done()
		}
	}()

	lockName := fmt.Sprintf("%s/%s/%s", ro.modelUUID, ro.appName, resName)
	locker := resourceMutex.Locker(lockName)
	locker.Lock()
	defer locker.Unlock()

	// Try and open the resource.
	res, reader, err := ro.open(resName)
	if err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return resource.Resource{}, nil, errors.Capture(err)
	} else if err == nil {
		// If the resource was stored on the controller, return immediately.
		return res, reader, nil
	}

	// The resource could not be opened, so may not be stored on the controller,
	// get the resource info and download from charmhub.
	res, err = ro.state.GetResource(ro.appName, resName)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	id := CharmID{
		URL:    ro.charmURL,
		Origin: ro.charmOrigin,
	}
	req := ResourceRequest{
		CharmID:  id,
		Name:     res.Name,
		Revision: res.Revision,
	}

	client, err := ro.resourceClientGetter(ctx)
	data, err := client.GetResource(req)
	if errors.Is(err, jujuerrors.NotFound) {
		// A NotFound error might not be detectable from some clients as the
		// error types may be lost after call, for example http. For these
		// cases, the next block will return un-annotated error.
		return resource.Resource{}, nil, errors.Errorf("getting resource from charmhub: %w", err)
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}
	res, reader, err = ro.set(data.Resource, data)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	return res, reader, nil
}

// set stores the resource info and data in a repo, if there is one.
// If no repo is in use then this is a no-op. Note that the returned
// reader may or may not be the same one that was passed in.
func (ro ResourceOpener) set(chRes charmresource.Resource, reader io.ReadCloser) (_ resource.Resource, _ io.ReadCloser, err error) {
	defer func() {
		if err != nil {
			// With no err, the reader was closed down in unitSetter Read().
			// Closing here with no error leads to a panic in Read, and the
			// unit's resource doc is never cleared of it's pending status.
			_ = reader.Close()
		}
	}()
	res, err := ro.state.SetResource(ro.appName, ro.retrievedBy.Id(), chRes, reader, state.DoNotIncrementCharmModifiedVersion)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	// Make sure to use the potentially updated resource details.
	reader, err := ro.resourceService.OpenResource()
	res, reader, err = ro.open(res.Name)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	return res, reader, nil
}

type resourceAccess struct {
	io.ReadCloser
	done func()
}

func (r *resourceAccess) Close() error {
	defer r.done()
	return r.ReadCloser.Close()
}

type ResourceRequest struct {
	// Channel is the channel from which to request the resource info.
	CharmID CharmID

	// Name is the name of the resource we're asking about.
	Name string

	// Revision is the specific revision of the resource we're asking about.
	Revision int
}

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	// Old style charm URLs are also supported i.e. charmstore.
	URL *charm.URL

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin state.CharmOrigin
}

// nopOpener is a type for creating no resource requests for accessing local
// charm resources.
type nopOpener struct{}

// newNopOpener creates a new nopOpener that creates a new resourceClient. The new
// nopClient performs no operations for getting resources.
func newNopOpener() resourceClientGetterFunc {
	no := &nopOpener{}
	return no.NewClient
}

// NewClient opens a new charmhub resourceClient.
func (o *nopOpener) NewClient(context.Context) (*ResourceRetryClient, error) {
	return newRetryClient(nopClient{}), nil
}

// nopClient implements a resourceClient for accessing resources from a given store,
// except this implementation performs no operations and instead returns a
// not-found error. This ensures that no outbound requests are used for
// scenarios covering local charms.
type nopClient struct{}

// GetResource is a no-op resourceClient implementation of a ResourceGetter. The
// implementation expects to never call the underlying resourceClient and instead
// returns a not-found error straight away.
func (nopClient) GetResource(req ResourceRequest) (ResourceData, error) {
	return ResourceData{}, jujuerrors.NotFoundf("resource %q", req.Name)
}
