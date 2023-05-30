// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/pack/pkg/buildpack (interfaces: BuildModule)

// Package testmocks is a generated GoMock package.
package testmocks

import (
	io "io"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	buildpack "github.com/buildpacks/pack/pkg/buildpack"
)

// MockBuildModule is a mock of BuildModule interface.
type MockBuildModule struct {
	ctrl     *gomock.Controller
	recorder *MockBuildModuleMockRecorder
}

// MockBuildModuleMockRecorder is the mock recorder for MockBuildModule.
type MockBuildModuleMockRecorder struct {
	mock *MockBuildModule
}

// NewMockBuildModule creates a new mock instance.
func NewMockBuildModule(ctrl *gomock.Controller) *MockBuildModule {
	mock := &MockBuildModule{ctrl: ctrl}
	mock.recorder = &MockBuildModuleMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBuildModule) EXPECT() *MockBuildModuleMockRecorder {
	return m.recorder
}

// ContainsFlattenedModules mocks base method.
func (m *MockBuildModule) ContainsFlattenedModules() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ContainsFlattenedModules")
	ret0, _ := ret[0].(bool)
	return ret0
}

// ContainsFlattenedModules indicates an expected call of ContainsFlattenedModules.
func (mr *MockBuildModuleMockRecorder) ContainsFlattenedModules() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ContainsFlattenedModules", reflect.TypeOf((*MockBuildModule)(nil).ContainsFlattenedModules))
}

// Descriptor mocks base method.
func (m *MockBuildModule) Descriptor() buildpack.Descriptor {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Descriptor")
	ret0, _ := ret[0].(buildpack.Descriptor)
	return ret0
}

// Descriptor indicates an expected call of Descriptor.
func (mr *MockBuildModuleMockRecorder) Descriptor() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Descriptor", reflect.TypeOf((*MockBuildModule)(nil).Descriptor))
}

// Open mocks base method.
func (m *MockBuildModule) Open() (io.ReadCloser, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Open")
	ret0, _ := ret[0].(io.ReadCloser)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Open indicates an expected call of Open.
func (mr *MockBuildModuleMockRecorder) Open() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Open", reflect.TypeOf((*MockBuildModule)(nil).Open))
}
