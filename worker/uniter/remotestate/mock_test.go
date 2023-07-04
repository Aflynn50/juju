// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"sync"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/domain"
)

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		stopped: make(chan struct{}),
	}
}

type mockWatcher struct {
	mu      sync.Mutex
	stopped chan struct{}
}

func (w *mockWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.Stopped() {
		close(w.stopped)
	}
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	return &mockNotifyWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan struct{}, 1),
	}
}

type mockNotifyWatcher struct {
	*mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func newMockStringsWatcher() *mockStringsWatcher {
	return &mockStringsWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan []string, 1),
	}
}

type mockStringsWatcher struct {
	*mockWatcher
	changes chan []string
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	return w.changes
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	return &mockRelationUnitsWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan watcher.RelationUnitsChange, 1),
	}
}

type mockRelationUnitsWatcher struct {
	*mockWatcher
	changes chan watcher.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}

type mockUniterClient struct {
	modelType                   model.ModelType
	unit                        mockUnit
	relations                   map[names.RelationTag]*mockRelation
	storageAttachment           map[params.StorageAttachmentId]params.StorageAttachment
	relationUnitsWatchers       map[names.RelationTag]*mockRelationUnitsWatcher
	relationAppWatchers         map[names.RelationTag]map[string]*mockNotifyWatcher
	storageAttachmentWatchers   map[names.StorageTag]*mockNotifyWatcher
	updateStatusInterval        time.Duration
	updateStatusIntervalWatcher *mockNotifyWatcher
	charm                       *mockCharm
}

func (m *mockUniterClient) Charm(*charm.URL) (domain.Charm, error) {
	if m.charm != nil {
		return m.charm, nil
	}
	return &mockCharm{}, nil
}

type mockCharm struct {
	domain.Charm
	required bool
}

func (c *mockCharm) LXDProfileRequired() (bool, error) {
	return c.required, nil
}

func (m *mockUniterClient) Relation(tag names.RelationTag) (domain.Relation, error) {
	r, ok := m.relations[tag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return r, nil
}

func (m *mockUniterClient) StorageAttachment(
	storageTag names.StorageTag, unitTag names.UnitTag,
) (params.StorageAttachment, error) {
	if unitTag != m.unit.tag {
		return params.StorageAttachment{}, errors.NewNotFound(&params.Error{Code: params.CodeNotFound}, "")
	}
	attachment, ok := m.storageAttachment[params.StorageAttachmentId{
		UnitTag:    unitTag.String(),
		StorageTag: storageTag.String(),
	}]
	if !ok {
		return params.StorageAttachment{}, errors.NewNotFound(&params.Error{Code: params.CodeNotFound}, "")
	}
	if attachment.Kind == params.StorageKindUnknown {
		return params.StorageAttachment{}, errors.NewNotProvisioned(&params.Error{Code: params.CodeNotProvisioned}, "")
	}
	return attachment, nil
}

func (m *mockUniterClient) StorageAttachmentLife(
	ids []params.StorageAttachmentId,
) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(ids))
	for i, id := range ids {
		attachment, ok := m.storageAttachment[id]
		if !ok {
			results[i] = params.LifeResult{
				Error: &params.Error{Code: params.CodeNotFound},
			}
			continue
		}
		results[i] = params.LifeResult{Life: attachment.Life}
	}
	return results, nil
}

func (m *mockUniterClient) Unit(tag names.UnitTag) (domain.Unit, error) {
	if tag != m.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return &m.unit, nil
}

func (m *mockUniterClient) WatchRelationUnits(
	relationTag names.RelationTag, unitTag names.UnitTag,
) (watcher.RelationUnitsWatcher, error) {
	if unitTag != m.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	watcher, ok := m.relationUnitsWatchers[relationTag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return watcher, nil
}

func (m *mockUniterClient) WatchStorageAttachment(
	storageTag names.StorageTag, unitTag names.UnitTag,
) (watcher.NotifyWatcher, error) {
	if unitTag != m.unit.tag {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	watcher, ok := m.storageAttachmentWatchers[storageTag]
	if !ok {
		return nil, &params.Error{Code: params.CodeNotFound}
	}
	return watcher, nil
}

func (m *mockUniterClient) UpdateStatusHookInterval() (time.Duration, error) {
	return m.updateStatusInterval, nil
}

func (m *mockUniterClient) WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error) {
	return m.updateStatusIntervalWatcher, nil
}

type mockUnit struct {
	domain.Unit
	tag                              names.UnitTag
	life                             life.Value
	providerID                       string
	resolved                         params.ResolvedMode
	application                      mockApplication
	unitWatcher                      *mockNotifyWatcher
	addressesWatcher                 *mockStringsWatcher
	configSettingsWatcher            *mockStringsWatcher
	applicationConfigSettingsWatcher *mockStringsWatcher
	upgradeSeriesWatcher             *mockNotifyWatcher
	storageWatcher                   *mockStringsWatcher
	actionWatcher                    *mockStringsWatcher
	relationsWatcher                 *mockStringsWatcher
	instanceDataWatcher              *mockNotifyWatcher
	lxdProfileName                   string
}

func (u *mockUnit) Life() life.Value {
	return u.life
}

func (u *mockUnit) LXDProfileName() (string, error) {
	return u.lxdProfileName, nil
}

func (u *mockUnit) Refresh() error {
	return nil
}

func (u *mockUnit) ProviderID() string {
	return u.providerID
}

func (u *mockUnit) Resolved() params.ResolvedMode {
	return u.resolved
}

func (u *mockUnit) Application() (domain.Application, error) {
	return &u.application, nil
}

func (u *mockUnit) Tag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) Watch() (watcher.NotifyWatcher, error) {
	return u.unitWatcher, nil
}

func (u *mockUnit) WatchAddressesHash() (watcher.StringsWatcher, error) {
	return u.addressesWatcher, nil
}

func (u *mockUnit) WatchConfigSettingsHash() (watcher.StringsWatcher, error) {
	return u.configSettingsWatcher, nil
}

func (u *mockUnit) WatchTrustConfigSettingsHash() (watcher.StringsWatcher, error) {
	return u.applicationConfigSettingsWatcher, nil
}

func (u *mockUnit) WatchStorage() (watcher.StringsWatcher, error) {
	return u.storageWatcher, nil
}

func (u *mockUnit) WatchActionNotifications() (watcher.StringsWatcher, error) {
	return u.actionWatcher, nil
}

func (u *mockUnit) WatchRelations() (watcher.StringsWatcher, error) {
	return u.relationsWatcher, nil
}

func (u *mockUnit) WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error) {
	return u.upgradeSeriesWatcher, nil
}

func (u *mockUnit) WatchInstanceData() (watcher.NotifyWatcher, error) {
	return u.instanceDataWatcher, nil
}

func (u *mockUnit) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, string, error) {
	return model.UpgradeSeriesPrepareStarted, "ubuntu@20.04", nil
}

func (u *mockUnit) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus, reason string) error {
	return nil
}

type mockApplication struct {
	domain.Application
	tag                   names.ApplicationTag
	life                  life.Value
	curl                  string
	charmModifiedVersion  int
	forceUpgrade          bool
	applicationWatcher    *mockNotifyWatcher
	leaderSettingsWatcher *mockNotifyWatcher
}

func (s *mockApplication) CharmModifiedVersion() (int, error) {
	return s.charmModifiedVersion, nil
}

func (s *mockApplication) CharmURL() (string, bool, error) {
	return s.curl, s.forceUpgrade, nil
}

func (s *mockApplication) Life() life.Value {
	return s.life
}

func (s *mockApplication) Refresh() error {
	return nil
}

func (s *mockApplication) Tag() names.ApplicationTag {
	return s.tag
}

func (s *mockApplication) Watch() (watcher.NotifyWatcher, error) {
	return s.applicationWatcher, nil
}

func (s *mockApplication) WatchLeadershipSettings() (watcher.NotifyWatcher, error) {
	return s.leaderSettingsWatcher, nil
}

type mockRelation struct {
	domain.Relation
	tag       names.RelationTag
	id        int
	life      life.Value
	suspended bool
}

func (r *mockRelation) Tag() names.RelationTag {
	return r.tag
}

func (r *mockRelation) Id() int {
	return r.id
}

func (r *mockRelation) Life() life.Value {
	return r.life
}

func (r *mockRelation) Suspended() bool {
	return r.suspended
}

func (r *mockRelation) UpdateSuspended(suspended bool) {
	r.suspended = suspended
}

type mockLeadershipTracker struct {
	leadership.Tracker
	claimTicket  mockTicket
	leaderTicket mockTicket
	minionTicket mockTicket
}

func (mock *mockLeadershipTracker) ClaimLeader() leadership.Ticket {
	return &mock.claimTicket
}

func (mock *mockLeadershipTracker) WaitLeader() leadership.Ticket {
	return &mock.leaderTicket
}

func (mock *mockLeadershipTracker) WaitMinion() leadership.Ticket {
	return &mock.minionTicket
}

type mockTicket struct {
	ch     chan struct{}
	result bool
}

func (t *mockTicket) Ready() <-chan struct{} {
	return t.ch
}

func (t *mockTicket) Wait() bool {
	return t.result
}

type mockSecretTriggerWatcher struct {
	ch     chan []string
	stopCh chan struct{}
}

func (w *mockSecretTriggerWatcher) Kill() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (*mockSecretTriggerWatcher) Wait() error {
	return nil
}

type mockSecretsClient struct {
	secretsWatcher          *mockStringsWatcher
	secretsRevisionsWatcher *mockStringsWatcher
	unitName                string
	owners                  []names.Tag
}

func (m *mockSecretsClient) WatchConsumedSecretsChanges(unitName string) (watcher.StringsWatcher, error) {
	m.unitName = unitName
	return m.secretsWatcher, nil
}

func (m *mockSecretsClient) GetConsumerSecretsRevisionInfo(unitName string, uris []string) (map[string]secrets.SecretRevisionInfo, error) {
	if unitName != m.unitName {
		return nil, errors.NotFoundf("unit %q", unitName)
	}
	result := make(map[string]secrets.SecretRevisionInfo)
	for i, uri := range uris {
		if i == 0 {
			continue
		}
		result[uri] = secrets.SecretRevisionInfo{
			Revision: 665 + i,
			Label:    "label-" + uri,
		}
	}
	return result, nil
}

func (m *mockSecretsClient) WatchObsolete(owners ...names.Tag) (watcher.StringsWatcher, error) {
	m.owners = owners
	return m.secretsRevisionsWatcher, nil
}
