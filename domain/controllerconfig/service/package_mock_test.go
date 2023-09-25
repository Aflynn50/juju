// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/domain/controllerconfig/service (interfaces: State,WatcherFactory)

// Package service is a generated GoMock package.
package service

import (
	context "context"
	reflect "reflect"

	changestream "github.com/juju/juju/core/changestream"
	watcher "github.com/juju/juju/core/watcher"
	gomock "go.uber.org/mock/gomock"
)

// MockState is a mock of State interface.
type MockState struct {
	ctrl     *gomock.Controller
	recorder *MockStateMockRecorder
}

// MockStateMockRecorder is the mock recorder for MockState.
type MockStateMockRecorder struct {
	mock *MockState
}

// NewMockState creates a new mock instance.
func NewMockState(ctrl *gomock.Controller) *MockState {
	mock := &MockState{ctrl: ctrl}
	mock.recorder = &MockStateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockState) EXPECT() *MockStateMockRecorder {
	return m.recorder
}

// AllKeysQuery mocks base method.
func (m *MockState) AllKeysQuery() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AllKeysQuery")
	ret0, _ := ret[0].(string)
	return ret0
}

// AllKeysQuery indicates an expected call of AllKeysQuery.
func (mr *MockStateMockRecorder) AllKeysQuery() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AllKeysQuery", reflect.TypeOf((*MockState)(nil).AllKeysQuery))
}

// ControllerConfig mocks base method.
func (m *MockState) ControllerConfig(arg0 context.Context) (map[string]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ControllerConfig", arg0)
	ret0, _ := ret[0].(map[string]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ControllerConfig indicates an expected call of ControllerConfig.
func (mr *MockStateMockRecorder) ControllerConfig(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ControllerConfig", reflect.TypeOf((*MockState)(nil).ControllerConfig), arg0)
}

// UpdateControllerConfig mocks base method.
func (m *MockState) UpdateControllerConfig(arg0 context.Context, arg1 map[string]string, arg2 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateControllerConfig", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateControllerConfig indicates an expected call of UpdateControllerConfig.
func (mr *MockStateMockRecorder) UpdateControllerConfig(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateControllerConfig", reflect.TypeOf((*MockState)(nil).UpdateControllerConfig), arg0, arg1, arg2)
}

// MockWatcherFactory is a mock of WatcherFactory interface.
type MockWatcherFactory struct {
	ctrl     *gomock.Controller
	recorder *MockWatcherFactoryMockRecorder
}

// MockWatcherFactoryMockRecorder is the mock recorder for MockWatcherFactory.
type MockWatcherFactoryMockRecorder struct {
	mock *MockWatcherFactory
}

// NewMockWatcherFactory creates a new mock instance.
func NewMockWatcherFactory(ctrl *gomock.Controller) *MockWatcherFactory {
	mock := &MockWatcherFactory{ctrl: ctrl}
	mock.recorder = &MockWatcherFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockWatcherFactory) EXPECT() *MockWatcherFactoryMockRecorder {
	return m.recorder
}

// NewNamespaceWatcher mocks base method.
func (m *MockWatcherFactory) NewNamespaceWatcher(arg0 string, arg1 changestream.ChangeType, arg2 string) (watcher.Watcher[[]string], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewNamespaceWatcher", arg0, arg1, arg2)
	ret0, _ := ret[0].(watcher.Watcher[[]string])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewNamespaceWatcher indicates an expected call of NewNamespaceWatcher.
func (mr *MockWatcherFactoryMockRecorder) NewNamespaceWatcher(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewNamespaceWatcher", reflect.TypeOf((*MockWatcherFactory)(nil).NewNamespaceWatcher), arg0, arg1, arg2)
}