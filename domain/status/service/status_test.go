// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

var now = time.Now()

func (s *statusSuite) TestEncodeCloudContainerStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.CloudContainerStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Running,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusRunning,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Running,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status:  status.CloudContainerStatusRunning,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeCloudContainerStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, test.output)
		result, err := decodeCloudContainerStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodeUnitAgentStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.UnitAgentStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Idle,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Allocating,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusAllocating,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Executing,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusExecuting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Failed,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusFailed,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Lost,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusLost,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Rebooting,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusRebooting,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeUnitAgentStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, test.output)
		result, err := decodeUnitAgentStatus(status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodingUnitAgentStatusError(c *gc.C) {
	output, err := encodeUnitAgentStatus(corestatus.StatusInfo{
		Status: corestatus.Error,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output, jc.DeepEquals, status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusError,
	})

}

func (s *statusSuite) TestDecodeUnitDisplayAndAgentStatus(c *gc.C) {
	agent, workload, err := decodeUnitDisplayAndAgentStatus(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Message: "hook failed: hook-name",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		},
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
				Since:  &now,
			},
			Present: true,
		},
		status.StatusInfo[status.CloudContainerStatusType]{
			Status: status.CloudContainerStatusUnset,
		})

	// If the agent is in an error state, the workload should also
	// be in an error state. In that case, the workload domain will
	// take precedence and we'll set the unit agent domain to idle.
	// This follows the same patter that already exists.

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agent, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Idle,
		Since:  &now,
	})
	c.Assert(workload, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Error,
		Since:   &now,
		Data:    map[string]interface{}{"foo": "bar"},
		Message: "hook failed: hook-name",
	})
}

func (s *statusSuite) TestEncodeWorkloadStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.WorkloadStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unset,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusUnset,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unknown,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusUnknown,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Maintenance,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Active,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Terminated,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusTerminated,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Active,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeWorkloadStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, test.output)
		result, err := decodeUnitWorkloadStatus(status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestUnitDisplayStatusWorkloadTerminatedBlockedMaintenanceDominates(c *gc.C) {
	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusBlocked,
	}

	workloadStatus := status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusTerminated,
			Message: "msg",
			Data:    []byte(`{"key":"value"}`),
			Since:   &now,
		},
		Present: true,
	}

	expected := corestatus.StatusInfo{
		Status:  corestatus.Terminated,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusBlocked
	expected.Status = corestatus.Blocked
	info, err = unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusMaintenance
	expected.Status = corestatus.Maintenance
	info, err = unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *statusSuite) TestUnitDisplayStatusContainerBlockedDominates(c *gc.C) {
	workloadStatus := status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusWaiting,
		},
		Present: true,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusBlocked,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusContainerWaitingDominatesActiveWorkload(c *gc.C) {
	workloadStatus := status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusActive,
		},
		Present: true,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusWaiting,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Waiting,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusContainerRunningDominatesWaitingWorkload(c *gc.C) {
	workloadStatus := status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusWaiting,
		},
		Present: true,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Running,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusDefaultsToWorkload(c *gc.C) {
	workloadStatus := status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "I'm an active workload",
		},
		Present: true,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "I'm a running container",
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "I'm an active workload",
	})
}

const (
	unitName1 = coreunit.Name("unit-1")
	unitName2 = coreunit.Name("unit-2")
	unitName3 = coreunit.Name("unit-3")
)

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsNoContainers(c *gc.C) {
	workloadStatuses := map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{
		unitName1: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Active,
	})

	info, err = applicationDisplayStatusFromUnits(
		workloadStatuses,
		make(map[coreunit.Name]status.StatusInfo[status.CloudContainerStatusType]),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Active,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsEmpty(c *gc.C) {
	info, err := applicationDisplayStatusFromUnits(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})

	info, err = applicationDisplayStatusFromUnits(
		map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{},
		map[coreunit.Name]status.StatusInfo[status.CloudContainerStatusType]{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceContainer(c *gc.C) {
	workloadStatuses := map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{
		unitName1: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]status.StatusInfo[status.CloudContainerStatusType]{
		unitName1: {Status: status.CloudContainerStatusRunning},
		unitName2: {Status: status.CloudContainerStatusBlocked},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceWorkload(c *gc.C) {
	workloadStatuses := map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{
		unitName1: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]status.StatusInfo[status.CloudContainerStatusType]{
		unitName1: {Status: status.CloudContainerStatusRunning},
		unitName2: {Status: status.CloudContainerStatusBlocked},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Maintenance,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPrioritisesUnitWithGreatestStatusPrecedence(c *gc.C) {
	workloadStatuses := map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{
		unitName1: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]status.StatusInfo[status.CloudContainerStatusType]{
		unitName1: {Status: status.CloudContainerStatusBlocked},
		unitName2: {Status: status.CloudContainerStatusRunning},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}
