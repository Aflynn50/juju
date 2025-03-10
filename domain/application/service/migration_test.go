// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainconstraints "github.com/juju/juju/domain/constraints"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationServiceSuite struct {
	baseSuite

	service *MigrationService
}

var _ = gc.Suite(&migrationServiceSuite{})

func (s *migrationServiceSuite) TestGetCharmIDWithoutRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name:   "foo",
		Source: domaincharm.CharmHubSource,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *migrationServiceSuite) TestGetCharmIDWithoutSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *migrationServiceSuite) TestGetCharmIDInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name: "Foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *migrationServiceSuite) TestGetCharmIDInvalidSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: ptr(42),
		Source:   "wrong-source",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmSourceNotValid)
}

func (s *migrationServiceSuite) TestGetCharmID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	rev := 42

	s.state.EXPECT().GetCharmID(gomock.Any(), "foo", rev, domaincharm.LocalSource).Return(id, nil)

	result, err := s.service.GetCharmID(context.Background(), domaincharm.GetCharmArgs{
		Name:     "foo",
		Revision: &rev,
		Source:   domaincharm.LocalSource,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, id)
}

func (s *migrationServiceSuite) TestGetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Conversion of the metadata tests is done in the types package.

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	metadata, locator, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata.Meta(), gc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, gc.Equals, domaincharm.CharmLocator{
		Source:   domaincharm.LocalSource,
		Revision: 42,
	})
}

func (s *migrationServiceSuite) TestGetCharmInvalidMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "blah",
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, `.*decode charm user.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidManifest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Manifest: domaincharm.Manifest{
			Bases: []domaincharm.Base{
				{
					Name: "foo",
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, `.*decode bases: decode base.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Actions: domaincharm.Actions{
			Actions: map[string]domaincharm.Action{
				"foo": {
					Params: []byte("!!!"),
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, `.*decode action params: unmarshal.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		Config: domaincharm.Config{
			Options: map[string]domaincharm.Option{
				"foo": {
					Type: "foo",
				},
			},
		},
		Source:    domaincharm.LocalSource,
		Revision:  42,
		Available: true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, `.*decode config.*`)
}

func (s *migrationServiceSuite) TestGetCharmInvalidLXDProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
		LXDProfile: []byte("!!!"),
		Source:     domaincharm.LocalSource,
		Revision:   42,
		Available:  true,
	}, nil, nil)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, `.*unmarshal lxd profile.*`)
}

func (s *migrationServiceSuite) TestGetCharmCharmNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharm(gomock.Any(), id).Return(domaincharm.Charm{}, nil, applicationerrors.CharmNotFound)

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *migrationServiceSuite) TestGetCharmInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetCharmByApplicationName(context.Background(), "")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) TestGetApplicationConfigAndSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  domaincharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"foo": "bar",
	})
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *migrationServiceSuite) TestGetApplicationConfigWithNameError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, errors.Errorf("boom"))

	_, _, err := s.service.GetApplicationConfigAndSettings(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, "boom")

}

func (s *migrationServiceSuite) TestGetApplicationConfigWithConfigError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, errors.Errorf("boom"))

	_, _, err := s.service.GetApplicationConfigAndSettings(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, "boom")

}

func (s *migrationServiceSuite) TestGetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{})
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{})
}

func (s *migrationServiceSuite) TestGetApplicationConfigNoConfigWithTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{
			Trust: true,
		}, nil)

	results, settings, err := s.service.GetApplicationConfigAndSettings(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{})
	c.Check(settings, gc.DeepEquals, application.ApplicationSettings{
		Trust: true,
	})
}

func (s *migrationServiceSuite) TestGetApplicationConfigInvalidApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationConfigAndSettings(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *migrationServiceSuite) TestImportApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	now := ptr(s.clock.Now())
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
		Manifest: s.minimalManifest(),
		Config: domaincharm.Config{
			Options: map[string]domaincharm.Option{
				"foo": {
					Type:    domaincharm.OptionString,
					Default: "baz",
				},
			},
		},
		ReferenceName: "ubuntu",
		Source:        domaincharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.ARM64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	downloadInfo := &domaincharm.DownloadInfo{
		Provenance:         domaincharm.ProvenanceDownload,
		DownloadURL:        "http://example.com",
		DownloadSize:       24,
		CharmhubIdentifier: "foobar",
	}

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)

	var receivedUnitArgs application.InsertUnitArg
	s.state.EXPECT().InsertUnit(gomock.Any(), coremodel.IAAS, id, gomock.Any()).DoAndReturn(func(_ context.Context, _ coremodel.ModelType, _ coreapplication.ID, args application.InsertUnitArg) error {
		receivedUnitArgs = args
		return nil
	})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{
		Options: map[string]charm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)

	args := application.AddApplicationArg{
		Charm:             ch,
		Platform:          platform,
		Scale:             1,
		CharmDownloadInfo: downloadInfo,
		Config: map[string]application.ApplicationConfig{
			"foo": {
				Type:  domaincharm.OptionString,
				Value: "bar",
			},
		},
		Settings: application.ApplicationSettings{
			Trust: true,
		},
		StorageParentDir: application.StorageParentDir,
	}
	s.state.EXPECT().CreateApplication(gomock.Any(), "ubuntu", args, nil).Return(id, nil)

	unitArg := ImportUnitArg{
		UnitName:     "ubuntu/666",
		PasswordHash: ptr("passwordhash"),
		AgentStatus: corestatus.StatusInfo{
			Status:  corestatus.Idle,
			Message: "agent status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   now,
		},
		WorkloadStatus: corestatus.StatusInfo{
			Status:  corestatus.Waiting,
			Message: "workload status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   now,
		},
		CloudContainer: nil,
	}

	cons := constraints.Value{
		Mem:      ptr(uint64(1024)),
		CpuPower: ptr(uint64(1000)),
		CpuCores: ptr(uint64(2)),
		Arch:     ptr("arm64"),
		Tags:     ptr([]string{"foo", "bar"}),
	}

	s.state.EXPECT().SetApplicationConstraints(gomock.Any(), id, domainconstraints.DecodeConstraints(cons)).Return(nil)

	err := s.service.ImportApplication(context.Background(), "ubuntu", ImportApplicationArgs{
		Charm: s.charm,
		CharmOrigin: corecharm.Origin{
			Source:   corecharm.CharmHub,
			Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
			Revision: ptr(42),
		},
		ApplicationConstraints: cons,
		ReferenceName:          "ubuntu",
		DownloadInfo:           downloadInfo,
		ApplicationConfig: map[string]any{
			"foo": "bar",
		},
		ApplicationSettings: application.ApplicationSettings{
			Trust: true,
		},
		Units: []ImportUnitArg{
			unitArg,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedUnitArgs := application.InsertUnitArg{
		UnitName:       "ubuntu/666",
		CloudContainer: nil,
		Password: ptr(application.PasswordInfo{
			PasswordHash:  "passwordhash",
			HashAlgorithm: 0,
		}),
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status:  application.UnitAgentStatusIdle,
				Message: "agent status",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: "workload status",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   now,
			},
		},
		StorageParentDir: application.StorageParentDir,
	}
	c.Check(receivedUnitArgs, gc.DeepEquals, expectedUnitArgs)
}

func (s *migrationServiceSuite) TestRemoveImportedApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.RemoveImportedApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.service = NewMigrationService(
		s.state,
		s.storageRegistryGetter,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}
