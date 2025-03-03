// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"context"

	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/ec2"
	coretesting "github.com/juju/juju/internal/testing"
)

type ProviderSuite struct {
	testing.IsolationSuite
	spec     environscloudspec.CloudSpec
	provider environs.EnvironProvider
}

var _ = gc.Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "foo",
			"secret-key": "bar",
		},
	)
	s.spec = environscloudspec.CloudSpec{
		Type:       "ec2",
		Name:       "aws",
		Region:     "us-east-1",
		Credential: &credential,
	}

	provider, err := environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
}

func (s *ProviderSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *ProviderSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *ProviderSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *ProviderSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: coretesting.ModelConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *ProviderSuite) TestVerifyCredentialsErrs(c *gc.C) {
	err := ec2.VerifyCredentials(context.Background(), nil)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(err, gc.Not(jc.ErrorIs), common.ErrorCredentialNotValid)
}

func (s *ProviderSuite) TestIsAuthorizationErrorIgnoresNil(c *gc.C) {
	isAuthErr := ec2.IsAuthorizationError(nil)
	c.Assert(isAuthErr, jc.IsFalse)
}

func (s *ProviderSuite) TestIsAuthorizationErrorConvertsCredentialRelatedFailures(c *gc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(isAuthErr, jc.IsTrue)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorConvertsCredentialRelatedFailuresWrapped(c *gc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			errors.Annotatef(&smithy.GenericAPIError{Code: code}, "wrapped"))
		c.Assert(isAuthErr, jc.IsTrue)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorNotInvalidCredential(c *gc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(isAuthErr, jc.IsFalse)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorHandlesOtherProviderErrors(c *gc.C) {
	// Any other ec2.Error is returned unwrapped.
	isAuthErr := ec2.IsAuthorizationError(&smithy.GenericAPIError{Code: "DryRunOperation"})
	c.Assert(isAuthErr, jc.IsFalse)
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsCredentialRelatedFailures(c *gc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		authErr := ec2.ConvertAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(authErr, jc.ErrorIs, common.ErrorCredentialNotValid)
	}
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsNotInvalidCredential(c *gc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		authErr := ec2.ConvertAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(authErr, gc.Not(jc.ErrorIs), common.ErrorCredentialNotValid)
	}
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsIsNil(c *gc.C) {
	authErr := ec2.ConvertAuthorizationError(nil)
	c.Assert(authErr, jc.ErrorIsNil)
}

func (s *ProviderSuite) TestConvertedCredentialError(c *gc.C) {
	// Trace() will keep error type
	inner := ec2.ConvertAuthorizationError(
		&smithy.GenericAPIError{Code: "Blocked"})
	traced := errors.Trace(inner)
	c.Assert(traced, gc.NotNil)
	c.Assert(traced, jc.ErrorIs, common.ErrorCredentialNotValid)

	// Annotate() will keep error type
	annotated := errors.Annotate(inner, "annotation")
	c.Assert(annotated, gc.NotNil)
	c.Assert(annotated, jc.ErrorIs, common.ErrorCredentialNotValid)

	// Running a CredentialNotValid through conversion call again is a no-op.
	again := ec2.ConvertAuthorizationError(inner)
	c.Assert(again, gc.NotNil)
	c.Assert(again, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(again.Error(), jc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")

	// Running an annotated CredentialNotValid through conversion call again is a no-op too.
	againAnotated := ec2.ConvertAuthorizationError(annotated)
	c.Assert(againAnotated, gc.NotNil)
	c.Assert(againAnotated, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(againAnotated.Error(), jc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")
}
