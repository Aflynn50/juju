// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	api "github.com/juju/juju/api/client/resources"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ResourceHandler is the HTTP handler for client downloads and
// uploads of resources.
type ResourceHandler struct {
	authFunc              func(*http.Request, ...string) (names.Tag, error)
	changeAllowedFunc     func(context.Context) error
	resourceServiceGetter ResourceServiceGetter
	logger                logger.Logger
}

// NewResourceHandler returns a new HTTP client resource handler.
func NewResourceHandler(
	authFunc func(*http.Request, ...string) (names.Tag, error),
	changeAllowedFunc func(context.Context) error,
	resourceServiceGetter ResourceServiceGetter,
	logger logger.Logger,
) *ResourceHandler {
	return &ResourceHandler{
		authFunc:              authFunc,
		changeAllowedFunc:     changeAllowedFunc,
		resourceServiceGetter: resourceServiceGetter,
		logger:                logger,
	}
}

// ServeHTTP implements http.Handler.
func (h *ResourceHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	tag, err := h.authFunc(req, names.UserTagKind, names.MachineTagKind, names.ControllerAgentTagKind, names.ApplicationTagKind)
	if err != nil {
		if err := internalhttp.SendError(resp, err, h.logger); err != nil {
			h.logger.Errorf(context.TODO(), "%v", err)
		}
		return
	}

	resourceService, err := h.resourceServiceGetter.Resource(req)
	if err != nil {
		if err := internalhttp.SendError(resp, err, h.logger); err != nil {
			h.logger.Errorf(context.TODO(), "returning error to user: %v", err)
		}
		return
	}

	switch req.Method {
	case "GET":
		reader, size, err := h.download(resourceService, req)
		if err != nil {
			if err := internalhttp.SendError(resp, err, h.logger); err != nil {
				h.logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		defer reader.Close()
		header := resp.Header()
		header.Set("Content-Type", params.ContentTypeRaw)
		header.Set("Content-Length", fmt.Sprint(size))
		resp.WriteHeader(http.StatusOK)
		if _, err := io.Copy(resp, reader); err != nil {
			h.logger.Errorf(context.TODO(), "resource download failed: %v", err)
		}
	case "PUT":
		if err := h.changeAllowedFunc(req.Context()); err != nil {
			if err := internalhttp.SendError(resp, err, h.logger); err != nil {
				h.logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		response, err := h.upload(resourceService, req, tagToUsername(tag))
		if err != nil {
			if err := internalhttp.SendError(resp, err, h.logger); err != nil {
				h.logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		if err := internalhttp.SendStatusAndJSON(resp, http.StatusOK, &response); err != nil {
			h.logger.Errorf(req.Context(), "%v", err)
		}
	default:
		if err := internalhttp.SendError(resp, jujuerrors.MethodNotAllowedf("unsupported method: %q", req.Method), h.logger); err != nil {
			h.logger.Errorf(context.TODO(), "%v", err)
		}
	}
}

func (h *ResourceHandler) download(service ResourceService, req *http.Request) (io.ReadCloser, int64, error) {
	query := req.URL.Query()
	application := query.Get(":application")
	name := query.Get(":resource")

	uuid, err := service.GetResourceUUIDByApplicationAndResourceName(req.Context(), application, name)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, 0, jujuerrors.NotFoundf("resource %s of application %s", name, application)
	} else if errors.Is(err, resourceerrors.ApplicationNotFound) {
		return nil, 0, jujuerrors.NotFoundf("application %s", application)
	} else if err != nil {
		return nil, 0, fmt.Errorf("getting resource uuid: %w", err)
	}

	res, reader, err := service.OpenResource(req.Context(), uuid)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, 0, jujuerrors.NotFoundf("resource %s of application %s", name, application)
	} else if errors.Is(err, resourceerrors.StoredResourceNotFound) {
		return nil, 0, jujuerrors.NotFoundf("resource %s of application %s has no blob downloaded on controller", name, application)
	} else if err != nil {
		return nil, 0, errors.Errorf("opening resource %s for application %s: %w", name, application, err)
	}
	return reader, res.Size, errors.Capture(err)
}

func (h *ResourceHandler) upload(service ResourceService, req *http.Request, username string) (*params.UploadResult, error) {
	uploaded, err := h.getUploadedResource(service, req)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = service.StoreResourceAndIncrementCharmModifiedVersion(
		req.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    uploaded.uuid,
			Reader:          req.Body,
			RetrievedBy:     username,
			RetrievedByType: coreresource.User,
			Size:            uploaded.size,
			Fingerprint:     uploaded.fingerprint,
			Origin:          charmresource.OriginUpload,
			Revision:        -1,
		},
	)
	if err != nil {
		return nil, errors.Errorf("storing resource %s of application %s: %w", uploaded.resourceName, uploaded.applicationName, err)
	}

	res, err := service.GetResource(req.Context(), uploaded.uuid)
	if err != nil {
		return nil, errors.Errorf("getting uploaded resource details: %w", err)
	}

	return &params.UploadResult{
		Resource: encodeResource(res),
	}, nil
}

// encodeResource converts a [coreresource.Resource] into
// a [params.Resource] struct.
func encodeResource(res coreresource.Resource) params.Resource {
	return params.Resource{
		CharmResource:   api.CharmResource2API(res.Resource),
		UUID:            res.UUID.String(),
		ApplicationName: res.ApplicationName,
		Username:        res.RetrievedBy,
		Timestamp:       res.Timestamp,
	}
}

// uploadedResource holds both the information about an uploaded
// resource and the reader containing its data.
type uploadedResource struct {
	// uuid is the resource UUID.
	uuid coreresource.UUID

	// applicationName is the Name of the application associated with the resource.
	applicationName string

	// resourceName is the name of the resource.
	resourceName string

	// size is the size of the resource blob.
	size int64

	// fingerprint is the hash of the resource blob.
	fingerprint charmresource.Fingerprint
}

// getUploadedResource reads the resource from the request, validates that it is
// known to the controller and validates the uploaded blobs contents.
func (h *ResourceHandler) getUploadedResource(resourceService ResourceService, req *http.Request) (*uploadedResource, error) {
	uReq, err := extractUploadRequest(req)
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuid, err := resourceService.GetResourceUUIDByApplicationAndResourceName(req.Context(), uReq.Application, uReq.Name)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, jujuerrors.NotFoundf("resource %s of application %s", uReq.Name, uReq.Application)
	} else if errors.Is(err, resourceerrors.ApplicationNotFound) {
		return nil, jujuerrors.NotFoundf("application %s", uReq.Application)
	} else if err != nil {
		return nil, errors.Errorf("getting resource uuid: %w", err)
	}

	res, err := resourceService.GetResource(req.Context(), uuid)
	if errors.Is(err, resourceerrors.ResourceNotFound) {
		return nil, jujuerrors.NotFoundf("resource %s of application %s", uReq.Name, uReq.Application)
	} else if errors.Is(err, resourceerrors.ApplicationNotFound) {
		return nil, jujuerrors.NotFoundf("application %s", uReq.Application)
	} else if err != nil {
		return nil, errors.Errorf("getting resource details: %w", err)
	}

	switch res.Type {
	case charmresource.TypeFile:
		ext := path.Ext(res.Path)
		if path.Ext(uReq.Filename) != ext {
			return nil, errors.Errorf("incorrect extension on resource upload %q, expected %q", uReq.Filename, ext)
		}
	}

	return &uploadedResource{
		uuid:            res.UUID,
		applicationName: uReq.Application,
		resourceName:    res.Resource.Name,
		size:            uReq.Size,
		fingerprint:     uReq.Fingerprint,
	}, nil
}

// extractUploadRequest pulls the required info from the HTTP request.
func extractUploadRequest(req *http.Request) (api.UploadRequest, error) {
	var ur api.UploadRequest

	ctype := req.Header.Get(api.HeaderContentType)
	if ctype != api.ContentTypeRaw {
		return ur, errors.Errorf("unsupported content type %q", ctype)
	}

	application, name := extractEndpointDetails(req.URL)
	fingerprint := req.Header.Get(api.HeaderContentSha384)

	fp, err := charmresource.ParseFingerprint(fingerprint)
	if err != nil {
		return ur, errors.Errorf("parsing fingerprint: %w", err)
	}

	filename, err := extractFilename(req)
	if err != nil {
		return ur, errors.Capture(err)
	}

	size, err := extractSize(req)
	if err != nil {
		return ur, errors.Capture(err)
	}

	ur = api.UploadRequest{
		Application: application,
		Name:        name,
		Filename:    filename,
		Size:        size,
		Fingerprint: fp,
	}
	return ur, nil
}

// extractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func extractEndpointDetails(url *url.URL) (application, name string) {
	application = url.Query().Get(":application")
	name = url.Query().Get(":resource")
	return application, name
}

func extractFilename(req *http.Request) (string, error) {
	disp := req.Header.Get(api.HeaderContentDisposition)

	// The first value returned here is the media type Name (e.g. "form-data"),
	// but we don't really care.
	_, vals, err := mime.ParseMediaType(disp)
	if err != nil {
		return "", errors.Errorf("badly formatted Content-Disposition: %w", err)
	}

	param, ok := vals[api.FilenameParamForContentDispositionHeader]
	if !ok {
		return "", errors.Errorf("missing %q in resource upload request",
			api.FilenameParamForContentDispositionHeader)
	}

	// Decode param, possibly encoded in base64.
	var filename string
	filename, err = new(mime.WordDecoder).Decode(param)
	if err != nil {
		// If encoding is not required, the encoder will return the original string.
		// However, the decoder doesn't expect that, so it barfs on non-encoded
		// strings. To detect if a string was not encoded, we simply try encoding
		// again, if it returns the same string, we know it wasn't encoded.
		if param == mime.BEncoding.Encode("utf-8", param) {
			filename = param
		} else {
			return "", errors.Errorf("decoding filename %q from upload request: %w", param, err)
		}
	}

	return filename, nil
}

func extractSize(req *http.Request) (int64, error) {
	var size int64
	if req.Header.Get(api.HeaderContentLength) == "" {
		size = req.ContentLength
		// size will be negative if there is no content.
		if size < 0 {
			size = 0
		}
		return size, nil
	}

	sizeRaw := req.Header.Get(api.HeaderContentLength)
	var err error
	size, err = strconv.ParseInt(sizeRaw, 10, 64)
	if err != nil {
		return 0, errors.Errorf("parsing size: %w", err)
	}
	return size, nil
}

func tagToUsername(tag names.Tag) string {
	switch tag := tag.(type) {
	case names.UserTag:
		return tag.Name()
	default:
		return ""
	}
}
