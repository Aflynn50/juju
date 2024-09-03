// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/worker/v4/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	coretesting "github.com/juju/juju/internal/testing"
)

var (
	// These vars hold the per-model workers we expect to run in
	// various circumstances. Note the absence of worker lists for
	// dying/dead states, because those states are not stable: if
	// they're working correctly the engine will be shut down.
	alwaysModelWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-responsible-flag",
		"not-alive-flag",
		"not-dead-flag",
		"provider-service-factories",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"valid-credential-flag",
	}
	requireValidCredentialModelWorkers = []string{
		"action-pruner",          // tertiary dependency: will be inactive because migration workers will be inactive
		"application-scaler",     // tertiary dependency: will be inactive because migration workers will be inactive
		"charm-downloader",       // tertiary dependency: will be inactive because migration workers will be inactive
		"charm-revision-updater", // tertiary dependency: will be inactive because migration workers will be inactive
		"machine-provisioner",
		"firewaller",
		"instance-mutater",
		"instance-poller",
		"logging-config-updater",  // tertiary dependency: will be inactive because migration workers will be inactive
		"machine-undertaker",      // tertiary dependency: will be inactive because migration workers will be inactive
		"migration-fortress",      // secondary dependency: will be inactive because depends on provider-upgrader
		"migration-inactive-flag", // secondary dependency: will be inactive because depends on provider-upgrader
		"migration-master",        // secondary dependency: will be inactive because depends on provider-upgrader
		"provider-tracker",
		"provider-upgrader",
		"remote-relations", // tertiary dependency: will be inactive because migration workers will be inactive
		"secrets-pruner",
		"state-cleaner",         // tertiary dependency: will be inactive because migration workers will be inactive
		"status-history-pruner", // tertiary dependency: will be inactive because migration workers will be inactive
		"storage-provisioner",   // tertiary dependency: will be inactive because migration workers will be inactive
		"undertaker",
		"unit-assigner", // tertiary dependency: will be inactive because migration workers will be inactive
		"user-secrets-drain-worker",
	}
	aliveModelWorkers = []string{
		"action-pruner",
		"application-scaler",
		"charm-downloader",
		"charm-revision-updater",
		"machine-provisioner",
		"firewaller",
		"instance-mutater",
		"instance-poller",
		"logging-config-updater",
		"machine-undertaker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"provider-tracker",
		"remote-relations",
		"secrets-pruner",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"unit-assigner",
		"user-secrets-drain-worker",
	}
	migratingModelWorkers = []string{
		"provider-tracker",
		"provider-upgrade-gate",
		"provider-upgraded-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
	}
	// ReallyLongTimeout should be long enough for the model-tracker
	// tests that depend on a hosted model; its backing state is not
	// accessible for StartSyncs, so we generally have to wait for at
	// least two 5s ticks to pass, and should expect rare circumstances
	// to take even longer.
	ReallyLongWait = coretesting.LongWait * 3

	alwaysMachineWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"broker-tracker",
		"charmhub-http-client",
		"clock",
		"instance-mutater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"state-config-watcher",
		"termination-signal-handler",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"valid-credential-flag",
	}
	notMigratingMachineWorkers = []string{
		"api-address-updater",
		"deployer",
		"disk-manager",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"is-not-controller-flag",
		"kvm-container-provisioner",
		"log-sender",
		"logging-config-updater",
		"lxd-container-provisioner",
		"machine-action-runner",
		"machiner",
		"proxy-config-updater",
		"reboot-executor",
		"ssh-authkeys-updater",
		"state-converter",
		"storage-provisioner",
		"upgrade-series",
	}
)

type ModelManifoldsFunc func(config model.ManifoldsConfig) dependency.Manifolds

func TrackModels(c *gc.C, tracker *agenttest.EngineTracker, inner ModelManifoldsFunc) ModelManifoldsFunc {
	return func(config model.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Model().Id()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}

type MachineManifoldsFunc func(config machine.ManifoldsConfig) dependency.Manifolds

func TrackMachines(c *gc.C, tracker *agenttest.EngineTracker, inner MachineManifoldsFunc) MachineManifoldsFunc {
	return func(config machine.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}
