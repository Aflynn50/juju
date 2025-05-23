// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/config"
	jujuversion "github.com/juju/juju/version"
)

// FakeAuthKeys holds the authorized key used for testing
// purposes in FakeConfig. It is valid for parsing with the utils/ssh
// authorized-key utilities.
const FakeAuthKeys = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAYQDP8fPSAMFm2PQGoVUks/FENVUMww1QTK6m++Y2qX9NGHm43kwEzxfoWR77wo6fhBhgFHsQ6ogE/cYLx77hOvjTchMEP74EVxSce0qtDjI7SwYbOpAButRId3g/Ef4STz8= joe@0.1.2.4`

func init() {
	_, err := ssh.ParseAuthorisedKey(FakeAuthKeys)
	if err != nil {
		panic("FakeAuthKeys does not hold a valid authorized key: " + err.Error())
	}
}

var (
	// FakeSupportedJujuSeries is used to provide a series of canned results
	// of series to test bootstrap code against.
	FakeSupportedJujuSeries = set.NewStrings("focal", "jammy", jujuversion.DefaultSupportedLTS())

	// FakeSupportedJujuBases is used to provide a list of canned results
	// of a base to test bootstrap code against.
	FakeSupportedJujuBases = []corebase.Base{
		corebase.MustParseBaseFromString("ubuntu@20.04"),
		corebase.MustParseBaseFromString("ubuntu@22.04"),
		corebase.MustParseBaseFromString("ubuntu@24.04"),
		jujuversion.DefaultSupportedLTSBase(),
	}
)

// FakeVersionNumber is a valid version number that can be used in testing.
var FakeVersionNumber = version.MustParse("2.99.0")

// ModelTag is a defined known valid UUID that can be used in testing.
var ModelTag = names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")

// ControllerTag is a defined known valid UUID that can be used in testing.
var ControllerTag = names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d")

// FakeControllerConfig returns an environment configuration
// that is expected to be found in state for a fake controller.
func FakeControllerConfig() controller.Config {
	return controller.Config{
		"controller-uuid":           ControllerTag.Id(),
		"ca-cert":                   CACert,
		"state-port":                1234,
		"api-port":                  17777,
		"set-numa-control-policy":   false,
		"model-logfile-max-backups": 1,
		"model-logfile-max-size":    "1M",
		"model-logs-size":           "1M",
		"max-txn-log-size":          "10M",
		"auditing-enabled":          false,
		"audit-log-capture-args":    true,
		"audit-log-max-size":        "200M",
		"audit-log-max-backups":     5,
		"query-tracing-threshold":   "1s",
	}
}

// FakeConfig returns an environment configuration for a
// fake provider with all required attributes set.
func FakeConfig() Attrs {
	return Attrs{
		"type":                      "someprovider",
		"name":                      "testmodel",
		"uuid":                      ModelTag.Id(),
		"authorized-keys":           FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"ssl-hostname-verification": true,
		"secret-backend":            "auto",
		"development":               false,
	}
}

// ModelConfig returns a default environment configuration suitable for
// setting in the state.
func ModelConfig(c *gc.C) *config.Config {
	uuid := mustUUID()
	return CustomModelConfig(c, Attrs{"uuid": uuid})
}

// mustUUID returns a stringified uuid or panics
func mustUUID() string {
	uuid, err := utils.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid.String()
}

// CustomModelConfig returns an environment configuration with
// additional specified keys added.
func CustomModelConfig(c *gc.C, extra Attrs) *config.Config {
	attrs := FakeConfig().Merge(Attrs{
		"agent-version": "2.0.0",
		"charmhub-url":  charmhub.DefaultServerURL,
	}).Merge(extra).Delete("admin-secret")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

const DefaultMongoPassword = "conn-from-name-secret"

// FakeJujuXDGDataHomeSuite isolates the user's home directory and
// sets up a Juju home with a sample environment and certificate.
type FakeJujuXDGDataHomeSuite struct {
	JujuOSEnvSuite
	testing.FakeHomeSuite
}

func (s *FakeJujuXDGDataHomeSuite) SetUpTest(c *gc.C) {
	s.JujuOSEnvSuite.SetUpTest(c)
	s.FakeHomeSuite.SetUpTest(c)
}

func (s *FakeJujuXDGDataHomeSuite) TearDownTest(c *gc.C) {
	s.FakeHomeSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

// AssertConfigParameterUpdated updates environment parameter and
// asserts that no errors were encountered.
func (s *FakeJujuXDGDataHomeSuite) AssertConfigParameterUpdated(c *gc.C, key, value string) {
	s.PatchEnvironment(key, value)
}
