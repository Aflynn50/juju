// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type newCharmResourceFunc func(c *gc.C, name, content string) charmresource.Resource

// NewResource produces full resource info for the given name and
// content. The origin is set to "upload". A reader is also returned
// which contains the content.
func NewResource(c *gc.C, stub *testing.Stub, name, applicationID, content string) resource.Opened {
	username := "a-user"
	return resource.Opened{
		Resource:   newResource(c, name, applicationID, username, content, NewCharmResource),
		ReadCloser: newStubReadCloser(stub, content),
	}
}

// NewDockerResource produces full resource info for the given name and
// content. The origin is set to "upload" (via resource created by  NewCharmDockerResource).
// A reader is also returned which contains the content.
func NewDockerResource(c *gc.C, stub *testing.Stub, name, applicationID, content string) resource.Opened {
	username := "a-user"
	return resource.Opened{
		Resource:   newResource(c, name, applicationID, username, content, NewCharmDockerResource),
		ReadCloser: newStubReadCloser(stub, content),
	}
}

// NewCharmResource produces basic resource info for the given name
// and content. The origin is set to "upload".
func NewCharmResource(c *gc.C, name, content string) charmresource.Resource {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeFile,
			Path:        name + ".tgz",
			Description: name + " description",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    0,
		Fingerprint: fp,
		Size:        int64(len(content)),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	return res
}

// NewCharmDockerResource produces basic docker resource info for the given name
// and content. The origin is set set to "upload".
func NewCharmDockerResource(c *gc.C, name, content string) charmresource.Resource {
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeContainerImage,
			Description: name + " description",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    0,
		Fingerprint: charmresource.Fingerprint{},
		Size:        0,
	}
	err := res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	return res
}

// NewPlaceholderResource returns resource info for a resource that
// has not been uploaded or pulled from the charm store yet. The origin
// is set to "upload".
func NewPlaceholderResource(c *gc.C, name, applicationID string) resource.Resource {
	res := newResource(c, name, applicationID, "", "", NewCharmResource)
	res.Fingerprint = charmresource.Fingerprint{}
	return res
}

func newResource(c *gc.C, name, applicationName, username, content string, charmResourceFunc newCharmResourceFunc) resource.Resource {
	var timestamp time.Time
	if username != "" {
		// TODO(perrito666) 2016-05-02 lp:1558657
		timestamp = time.Now().UTC()
	}
	uuid, err := resource.NewUUID()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot generate uuid for resource %q ", name))

	res := resource.Resource{
		Resource:        charmResourceFunc(c, name, content),
		UUID:            uuid,
		ApplicationName: applicationName,
		RetrievedBy:     username,
		Timestamp:       timestamp,
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}

type stubReadCloser struct {
	io.Reader
	io.Closer
}

func newStubReadCloser(stub *testing.Stub, content string) io.ReadCloser {
	return &stubReadCloser{
		Reader: filetesting.NewStubReader(stub, content),
		Closer: &filetesting.StubCloser{
			Stub: stub,
		},
	}
}

// GenResourceUUID can be used in testing for generating a resource UUID that is
// checked for subsequent errors using the test suit's go check instance.
func GenResourceUUID(c *gc.C) resource.UUID {
	id, err := resource.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return id
}
