// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/buildpacks/pack/internal/commands (interfaces: PackClient)

// Package testmocks is a generated GoMock package.
package testmocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"

	client "github.com/buildpacks/pack/pkg/client"
)

// MockPackClient is a mock of PackClient interface.
type MockPackClient struct {
	ctrl     *gomock.Controller
	recorder *MockPackClientMockRecorder
}

// BuildWithBuildkit implements commands.PackClient.
func (m *MockPackClient) BuildWithBuildkit(context.Context, client.BuildOptions) error {
	panic("unimplemented")
}

// MockPackClientMockRecorder is the mock recorder for MockPackClient.
type MockPackClientMockRecorder struct {
	mock *MockPackClient
}

// NewMockPackClient creates a new mock instance.
func NewMockPackClient(ctrl *gomock.Controller) *MockPackClient {
	mock := &MockPackClient{ctrl: ctrl}
	mock.recorder = &MockPackClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPackClient) EXPECT() *MockPackClientMockRecorder {
	return m.recorder
}

// Build mocks base method.
func (m *MockPackClient) Build(arg0 context.Context, arg1 client.BuildOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Build", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Build indicates an expected call of Build.
func (mr *MockPackClientMockRecorder) Build(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Build", reflect.TypeOf((*MockPackClient)(nil).Build), arg0, arg1)
}

// CreateBuilder mocks base method.
func (m *MockPackClient) CreateBuilder(arg0 context.Context, arg1 client.CreateBuilderOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateBuilder", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateBuilder indicates an expected call of CreateBuilder.
func (mr *MockPackClientMockRecorder) CreateBuilder(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateBuilder", reflect.TypeOf((*MockPackClient)(nil).CreateBuilder), arg0, arg1)
}

// DownloadSBOM mocks base method.
func (m *MockPackClient) DownloadSBOM(arg0 string, arg1 client.DownloadSBOMOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DownloadSBOM", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DownloadSBOM indicates an expected call of DownloadSBOM.
func (mr *MockPackClientMockRecorder) DownloadSBOM(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DownloadSBOM", reflect.TypeOf((*MockPackClient)(nil).DownloadSBOM), arg0, arg1)
}

// InspectBuilder mocks base method.
func (m *MockPackClient) InspectBuilder(arg0 string, arg1 bool, arg2 ...client.BuilderInspectionModifier) (*client.BuilderInfo, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "InspectBuilder", varargs...)
	ret0, _ := ret[0].(*client.BuilderInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InspectBuilder indicates an expected call of InspectBuilder.
func (mr *MockPackClientMockRecorder) InspectBuilder(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InspectBuilder", reflect.TypeOf((*MockPackClient)(nil).InspectBuilder), varargs...)
}

// InspectBuildpack mocks base method.
func (m *MockPackClient) InspectBuildpack(arg0 client.InspectBuildpackOptions) (*client.BuildpackInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InspectBuildpack", arg0)
	ret0, _ := ret[0].(*client.BuildpackInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InspectBuildpack indicates an expected call of InspectBuildpack.
func (mr *MockPackClientMockRecorder) InspectBuildpack(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InspectBuildpack", reflect.TypeOf((*MockPackClient)(nil).InspectBuildpack), arg0)
}

// InspectExtension mocks base method.
func (m *MockPackClient) InspectExtension(arg0 client.InspectExtensionOptions) (*client.ExtensionInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InspectExtension", arg0)
	ret0, _ := ret[0].(*client.ExtensionInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InspectExtension indicates an expected call of InspectExtension.
func (mr *MockPackClientMockRecorder) InspectExtension(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InspectExtension", reflect.TypeOf((*MockPackClient)(nil).InspectExtension), arg0)
}

// InspectImage mocks base method.
func (m *MockPackClient) InspectImage(arg0 string, arg1 bool) (*client.ImageInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InspectImage", arg0, arg1)
	ret0, _ := ret[0].(*client.ImageInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InspectImage indicates an expected call of InspectImage.
func (mr *MockPackClientMockRecorder) InspectImage(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InspectImage", reflect.TypeOf((*MockPackClient)(nil).InspectImage), arg0, arg1)
}

// NewBuildpack mocks base method.
func (m *MockPackClient) NewBuildpack(arg0 context.Context, arg1 client.NewBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewBuildpack", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// NewBuildpack indicates an expected call of NewBuildpack.
func (mr *MockPackClientMockRecorder) NewBuildpack(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewBuildpack", reflect.TypeOf((*MockPackClient)(nil).NewBuildpack), arg0, arg1)
}

// PackageBuildpack mocks base method.
func (m *MockPackClient) PackageBuildpack(arg0 context.Context, arg1 client.PackageBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackageBuildpack", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// PackageBuildpack indicates an expected call of PackageBuildpack.
func (mr *MockPackClientMockRecorder) PackageBuildpack(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PackageBuildpack", reflect.TypeOf((*MockPackClient)(nil).PackageBuildpack), arg0, arg1)
}

// PackageExtension mocks base method.
func (m *MockPackClient) PackageExtension(arg0 context.Context, arg1 client.PackageBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PackageExtension", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// PackageExtension indicates an expected call of PackageExtension.
func (mr *MockPackClientMockRecorder) PackageExtension(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PackageExtension", reflect.TypeOf((*MockPackClient)(nil).PackageExtension), arg0, arg1)
}

// PullBuildpack mocks base method.
func (m *MockPackClient) PullBuildpack(arg0 context.Context, arg1 client.PullBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PullBuildpack", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// PullBuildpack indicates an expected call of PullBuildpack.
func (mr *MockPackClientMockRecorder) PullBuildpack(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PullBuildpack", reflect.TypeOf((*MockPackClient)(nil).PullBuildpack), arg0, arg1)
}

// Rebase mocks base method.
func (m *MockPackClient) Rebase(arg0 context.Context, arg1 client.RebaseOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Rebase", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Rebase indicates an expected call of Rebase.
func (mr *MockPackClientMockRecorder) Rebase(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Rebase", reflect.TypeOf((*MockPackClient)(nil).Rebase), arg0, arg1)
}

// RegisterBuildpack mocks base method.
func (m *MockPackClient) RegisterBuildpack(arg0 context.Context, arg1 client.RegisterBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterBuildpack", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// RegisterBuildpack indicates an expected call of RegisterBuildpack.
func (mr *MockPackClientMockRecorder) RegisterBuildpack(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterBuildpack", reflect.TypeOf((*MockPackClient)(nil).RegisterBuildpack), arg0, arg1)
}

// YankBuildpack mocks base method.
func (m *MockPackClient) YankBuildpack(arg0 client.YankBuildpackOptions) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "YankBuildpack", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// YankBuildpack indicates an expected call of YankBuildpack.
func (mr *MockPackClientMockRecorder) YankBuildpack(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "YankBuildpack", reflect.TypeOf((*MockPackClient)(nil).YankBuildpack), arg0)
}
