// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/pack/internal/asset (interfaces: URICacheFetcher)

// Package testmocks is a generated GoMock package.
package testmocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	ocipackage "github.com/buildpacks/pack/internal/ocipackage"
)

// MockURICacheFetcher is a mock of URICacheFetcher interface
type MockURICacheFetcher struct {
	ctrl     *gomock.Controller
	recorder *MockURICacheFetcherMockRecorder
}

// MockURICacheFetcherMockRecorder is the mock recorder for MockURICacheFetcher
type MockURICacheFetcherMockRecorder struct {
	mock *MockURICacheFetcher
}

// NewMockURICacheFetcher creates a new mock instance
func NewMockURICacheFetcher(ctrl *gomock.Controller) *MockURICacheFetcher {
	mock := &MockURICacheFetcher{ctrl: ctrl}
	mock.recorder = &MockURICacheFetcherMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockURICacheFetcher) EXPECT() *MockURICacheFetcherMockRecorder {
	return m.recorder
}

// FetchURIAssets mocks base method
func (m *MockURICacheFetcher) FetchURIAssets(arg0 context.Context, arg1 ...string) ([]*ocipackage.OciLayoutPackage, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "FetchURIAssets", varargs...)
	ret0, _ := ret[0].([]*ocipackage.OciLayoutPackage)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchURIAssets indicates an expected call of FetchURIAssets
func (mr *MockURICacheFetcherMockRecorder) FetchURIAssets(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchURIAssets", reflect.TypeOf((*MockURICacheFetcher)(nil).FetchURIAssets), varargs...)
}
