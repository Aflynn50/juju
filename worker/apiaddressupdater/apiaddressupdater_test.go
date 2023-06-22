// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apiaddressupdater/mocks"
)

type APIAddressUpdaterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&APIAddressUpdaterSuite{})

func (s *APIAddressUpdaterSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&network.AddressesForInterfaceName, func(string) ([]string, error) {
		return nil, nil
	})
}

type apiAddressSetter struct {
	servers chan []corenetwork.HostPorts
	err     error
}

func (s *apiAddressSetter) SetAPIHostPorts(servers []corenetwork.HostPorts) error {
	s.servers <- servers
	return s.err
}

func (s *APIAddressUpdaterSuite) TestStartStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockAPIAddresser(ctrl)
	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	client.EXPECT().WatchAPIHostPorts().Return(watch, nil)

	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: client,
			Setter:    &apiAddressSetter{},
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, worker)
}

func (s *APIAddressUpdaterSuite) assertInitialUpdate(c *gc.C, ctrl *gomock.Controller, setter *apiAddressSetter) (worker.Worker, *mocks.MockAPIAddresser, chan struct{}) {
	ch := make(chan struct{}, 1)
	watch := watchertest.NewMockNotifyWatcher(ch)
	ch <- struct{}{}

	result := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}

	client := mocks.NewMockAPIAddresser(ctrl)
	client.EXPECT().WatchAPIHostPorts().Return(watch, nil).MinTimes(1)
	client.EXPECT().APIHostPorts().Return([]corenetwork.ProviderHostPorts{result}, nil)

	w, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: client,
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)

	expServer := result.HostPorts()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}

	// The values are also available through the report.
	reporter, ok := w.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"servers": [][]string{{"localhost:1234", "127.0.0.1:1234"}},
	})
	return w, client, ch
}

func (s *APIAddressUpdaterSuite) TestAddressInitialUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	w, _, _ := s.assertInitialUpdate(c, ctrl, setter)
	defer workertest.CleanKill(c, w)

}

func (s *APIAddressUpdaterSuite) TestAddressChange(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	w, client, ch := s.assertInitialUpdate(c, ctrl, setter)
	defer workertest.CleanKill(c, w)

	result := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.0.1").AsProviderAddress(), NetPort: 1234},
	}

	client.EXPECT().APIHostPorts().Return([]corenetwork.ProviderHostPorts{result}, nil)

	ch <- struct{}{}

	expServer := result.HostPorts()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestAddressChangeEmpty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	w, client, ch := s.assertInitialUpdate(c, ctrl, setter)
	defer workertest.CleanKill(c, w)

	client.EXPECT().APIHostPorts().Return([]corenetwork.ProviderHostPorts{}, nil)

	ch <- struct{}{}

	expServer := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}.HostPorts()

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for initial update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func toProviderHostPorts(hps corenetwork.SpaceHostPorts) corenetwork.ProviderHostPorts {
	pHPs := make(corenetwork.ProviderHostPorts, len(hps))
	for i, hp := range hps {
		pHPs[i] = corenetwork.ProviderHostPort{
			ProviderAddress: corenetwork.ProviderAddress{MachineAddress: hp.MachineAddress},
			NetPort:         hp.NetPort,
		}
	}
	return pHPs
}

func (s *APIAddressUpdaterSuite) TestBridgeAddressesFiltering(c *gc.C) {
	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.4.4",
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []string{
				"192.168.122.1",
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	initialServers := []corenetwork.ProviderHostPorts{
		toProviderHostPorts(corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1")),
		toProviderHostPorts(corenetwork.NewSpaceHostPorts(
			4321,
			"10.0.3.3",      // not filtered
			"10.0.4.1",      // filtered lxd bridge address
			"10.0.4.2",      // not filtered
			"192.168.122.1", // filtered default virbr0
		)),
	}

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	w, client, ch := s.assertInitialUpdate(c, ctrl, setter)
	defer workertest.CleanKill(c, w)

	client.EXPECT().APIHostPorts().Return(initialServers, nil)

	ch <- struct{}{}

	updatedServers := []corenetwork.ProviderHostPorts{
		toProviderHostPorts(corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1")),
		toProviderHostPorts(corenetwork.NewSpaceHostPorts(
			4001,
			"10.0.3.3", // not filtered
		)),
	}

	expServer1 := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value, and
	// then the updated value, but filtering occurs in both cases.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerInit := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4321},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.4.2").AsProviderAddress(), NetPort: 4321},
		}.HostPorts()
		c.Assert(servers, jc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerInit})
	}

	client.EXPECT().APIHostPorts().Return(updatedServers, nil)

	ch <- struct{}{}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerUpd := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4001},
		}.HostPorts()
		c.Assert(servers, jc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerUpd})
	}
}

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *gc.C) {
	err := validConfig().Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingAddresser(c *gc.C) {
	config := validConfig()
	config.Addresser = nil
	checkNotValid(c, config, "nil Addresser not valid")
}

func (*ValidateSuite) TestMissingSetter(c *gc.C) {
	config := validConfig()
	config.Setter = nil
	checkNotValid(c, config, "nil Setter not valid")
}

func (*ValidateSuite) TestMissingLogger(c *gc.C) {
	config := validConfig()
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}

func validConfig() apiaddressupdater.Config {
	return apiaddressupdater.Config{
		Addresser: struct{ apiaddressupdater.APIAddresser }{},
		Setter: struct {
			apiaddressupdater.APIAddressSetter
		}{},
		Logger: loggo.GetLogger("test"),
	}
}

func checkNotValid(c *gc.C, config apiaddressupdater.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := apiaddressupdater.NewAPIAddressUpdater(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
