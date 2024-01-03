// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/testing"
)

type mockAPIConnection struct {
	api.Connection

	// If non-nil, close is called when the Close method is called.
	close func(api.Connection) error

	addr          string
	ipAddr        string
	apiHostPorts  []network.MachineHostPorts
	modelTag      string
	controllerTag string
	publicDNSName string
}

type mockedStateFlags int

const (
	noFlags        mockedStateFlags = 0x0000
	mockedHostPort mockedStateFlags = 0x0001
	mockedModelTag mockedStateFlags = 0x0002
)

// mockedAPIState returns a mocked-up implementation
// of api.Connection. The logical OR of the flags specifies
// whether to include a fake host port and model tag
// in the result.
func mockedAPIState(flags mockedStateFlags) *mockAPIConnection {
	hasHostPort := flags&mockedHostPort == mockedHostPort
	hasModelTag := flags&mockedModelTag == mockedModelTag
	addr := ""

	apiHostPorts := []network.MachineHostPorts{}
	if hasHostPort {
		apiHostPorts = []network.MachineHostPorts{{
			network.MachineHostPort{MachineAddress: network.NewMachineAddress("0.1.2.3"), NetPort: network.NetPort(1234)},
			network.MachineHostPort{MachineAddress: network.NewMachineAddress("2001:db8::1"), NetPort: network.NetPort(1234)},
		}}
	}
	modelTag := ""
	if hasModelTag {
		modelTag = "model-df136476-12e9-11e4-8a70-b2227cce2b54"
	}
	return &mockAPIConnection{
		apiHostPorts:  apiHostPorts,
		modelTag:      modelTag,
		controllerTag: testing.ControllerTag.Id(),
		addr:          addr,
	}
}

func (s *mockAPIConnection) Close() error {
	if s.close != nil {
		return s.close(s)
	}
	return nil
}

func (s *mockAPIConnection) ServerVersion() (version.Number, bool) {
	return version.MustParse("1.2.3"), true
}

func (s *mockAPIConnection) IPAddr() string {
	return s.ipAddr
}

func (s *mockAPIConnection) Addr() string {
	return s.addr
}

func (s *mockAPIConnection) PublicDNSName() string {
	return s.publicDNSName
}

func (s *mockAPIConnection) APIHostPorts() []network.MachineHostPorts {
	return s.apiHostPorts
}

func (s *mockAPIConnection) ModelTag() (names.ModelTag, bool) {
	if s.modelTag == "" {
		return names.ModelTag{}, false
	}
	t, err := names.ParseModelTag(s.modelTag)
	if err != nil {
		panic("bad model tag")
	}
	return t, true
}

func (s *mockAPIConnection) ControllerTag() names.ControllerTag {
	t, err := names.ParseControllerTag(s.controllerTag)
	if err != nil {
		panic("bad controller tag")
	}
	return t
}

func (s *mockAPIConnection) AuthTag() names.Tag {
	return names.NewUserTag("admin")
}

func (s *mockAPIConnection) ControllerAccess() string {
	return "superuser"
}

func panicAPIOpen(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
	panic("api.Open called unexpectedly")
}
