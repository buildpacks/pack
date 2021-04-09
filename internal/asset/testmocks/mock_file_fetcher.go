// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/pack/internal/asset (interfaces: FileCacheFetcher)

// Package testmocks is a generated GoMock package.
package testmocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	ocipackage "github.com/buildpacks/pack/internal/ocipackage"
)

// MockFileCacheFetcher is a mock of FileCacheFetcher interface
type MockFileCacheFetcher struct {
	ctrl     *gomock.Controller
	recorder *MockFileCacheFetcherMockRecorder
}

// MockFileCacheFetcherMockRecorder is the mock recorder for MockFileCacheFetcher
type MockFileCacheFetcherMockRecorder struct {
	mock *MockFileCacheFetcher
}

// NewMockFileCacheFetcher creates a new mock instance
func NewMockFileCacheFetcher(ctrl *gomock.Controller) *MockFileCacheFetcher {
	mock := &MockFileCacheFetcher{ctrl: ctrl}
	mock.recorder = &MockFileCacheFetcherMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockFileCacheFetcher) EXPECT() *MockFileCacheFetcherMockRecorder {
	return m.recorder
}

// FetchFileAssets mocks base method
func (m *MockFileCacheFetcher) FetchFileAssets(arg0 context.Context, arg1 string, arg2 ...string) ([]*ocipackage.OciLayoutPackage, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "FetchFileAssets", varargs...)
	ret0, _ := ret[0].([]*ocipackage.OciLayoutPackage)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchFileAssets indicates an expected call of FetchFileAssets
func (mr *MockFileCacheFetcherMockRecorder) FetchFileAssets(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchFileAssets", reflect.TypeOf((*MockFileCacheFetcher)(nil).FetchFileAssets), varargs...)
}
