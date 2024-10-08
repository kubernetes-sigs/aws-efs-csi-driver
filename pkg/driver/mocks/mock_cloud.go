// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/cloud/cloud.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	efs "github.com/aws/aws-sdk-go-v2/service/efs"
	gomock "github.com/golang/mock/gomock"
	cloud "github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

// MockEfs is a mock of Efs interface.
type MockEfs struct {
	ctrl     *gomock.Controller
	recorder *MockEfsMockRecorder
}

// MockEfsMockRecorder is the mock recorder for MockEfs.
type MockEfsMockRecorder struct {
	mock *MockEfs
}

// NewMockEfs creates a new mock instance.
func NewMockEfs(ctrl *gomock.Controller) *MockEfs {
	mock := &MockEfs{ctrl: ctrl}
	mock.recorder = &MockEfsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockEfs) EXPECT() *MockEfsMockRecorder {
	return m.recorder
}

// CreateAccessPoint mocks base method.
func (m *MockEfs) CreateAccessPoint(arg0 context.Context, arg1 *efs.CreateAccessPointInput, arg2 ...func(*efs.Options)) (*efs.CreateAccessPointOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CreateAccessPoint", varargs...)
	ret0, _ := ret[0].(*efs.CreateAccessPointOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateAccessPoint indicates an expected call of CreateAccessPoint.
func (mr *MockEfsMockRecorder) CreateAccessPoint(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateAccessPoint", reflect.TypeOf((*MockEfs)(nil).CreateAccessPoint), varargs...)
}

// DeleteAccessPoint mocks base method.
func (m *MockEfs) DeleteAccessPoint(arg0 context.Context, arg1 *efs.DeleteAccessPointInput, arg2 ...func(*efs.Options)) (*efs.DeleteAccessPointOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DeleteAccessPoint", varargs...)
	ret0, _ := ret[0].(*efs.DeleteAccessPointOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteAccessPoint indicates an expected call of DeleteAccessPoint.
func (mr *MockEfsMockRecorder) DeleteAccessPoint(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteAccessPoint", reflect.TypeOf((*MockEfs)(nil).DeleteAccessPoint), varargs...)
}

// DescribeAccessPoints mocks base method.
func (m *MockEfs) DescribeAccessPoints(arg0 context.Context, arg1 *efs.DescribeAccessPointsInput, arg2 ...func(*efs.Options)) (*efs.DescribeAccessPointsOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeAccessPoints", varargs...)
	ret0, _ := ret[0].(*efs.DescribeAccessPointsOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeAccessPoints indicates an expected call of DescribeAccessPoints.
func (mr *MockEfsMockRecorder) DescribeAccessPoints(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeAccessPoints", reflect.TypeOf((*MockEfs)(nil).DescribeAccessPoints), varargs...)
}

// DescribeFileSystems mocks base method.
func (m *MockEfs) DescribeFileSystems(arg0 context.Context, arg1 *efs.DescribeFileSystemsInput, arg2 ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeFileSystems", varargs...)
	ret0, _ := ret[0].(*efs.DescribeFileSystemsOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeFileSystems indicates an expected call of DescribeFileSystems.
func (mr *MockEfsMockRecorder) DescribeFileSystems(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeFileSystems", reflect.TypeOf((*MockEfs)(nil).DescribeFileSystems), varargs...)
}

// DescribeMountTargets mocks base method.
func (m *MockEfs) DescribeMountTargets(arg0 context.Context, arg1 *efs.DescribeMountTargetsInput, arg2 ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeMountTargets", varargs...)
	ret0, _ := ret[0].(*efs.DescribeMountTargetsOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeMountTargets indicates an expected call of DescribeMountTargets.
func (mr *MockEfsMockRecorder) DescribeMountTargets(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeMountTargets", reflect.TypeOf((*MockEfs)(nil).DescribeMountTargets), varargs...)
}

// MockCloud is a mock of Cloud interface.
type MockCloud struct {
	ctrl     *gomock.Controller
	recorder *MockCloudMockRecorder
}

// MockCloudMockRecorder is the mock recorder for MockCloud.
type MockCloudMockRecorder struct {
	mock *MockCloud
}

// NewMockCloud creates a new mock instance.
func NewMockCloud(ctrl *gomock.Controller) *MockCloud {
	mock := &MockCloud{ctrl: ctrl}
	mock.recorder = &MockCloudMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCloud) EXPECT() *MockCloudMockRecorder {
	return m.recorder
}

// CreateAccessPoint mocks base method.
func (m *MockCloud) CreateAccessPoint(ctx context.Context, clientToken string, accessPointOpts *cloud.AccessPointOptions) (*cloud.AccessPoint, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateAccessPoint", ctx, clientToken, accessPointOpts)
	ret0, _ := ret[0].(*cloud.AccessPoint)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateAccessPoint indicates an expected call of CreateAccessPoint.
func (mr *MockCloudMockRecorder) CreateAccessPoint(ctx, clientToken, accessPointOpts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateAccessPoint", reflect.TypeOf((*MockCloud)(nil).CreateAccessPoint), ctx, clientToken, accessPointOpts)
}

// DeleteAccessPoint mocks base method.
func (m *MockCloud) DeleteAccessPoint(ctx context.Context, accessPointId string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteAccessPoint", ctx, accessPointId)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteAccessPoint indicates an expected call of DeleteAccessPoint.
func (mr *MockCloudMockRecorder) DeleteAccessPoint(ctx, accessPointId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteAccessPoint", reflect.TypeOf((*MockCloud)(nil).DeleteAccessPoint), ctx, accessPointId)
}

// DescribeAccessPoint mocks base method.
func (m *MockCloud) DescribeAccessPoint(ctx context.Context, accessPointId string) (*cloud.AccessPoint, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeAccessPoint", ctx, accessPointId)
	ret0, _ := ret[0].(*cloud.AccessPoint)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeAccessPoint indicates an expected call of DescribeAccessPoint.
func (mr *MockCloudMockRecorder) DescribeAccessPoint(ctx, accessPointId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeAccessPoint", reflect.TypeOf((*MockCloud)(nil).DescribeAccessPoint), ctx, accessPointId)
}

// DescribeFileSystem mocks base method.
func (m *MockCloud) DescribeFileSystem(ctx context.Context, fileSystemId string) (*cloud.FileSystem, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeFileSystem", ctx, fileSystemId)
	ret0, _ := ret[0].(*cloud.FileSystem)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeFileSystem indicates an expected call of DescribeFileSystem.
func (mr *MockCloudMockRecorder) DescribeFileSystem(ctx, fileSystemId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeFileSystem", reflect.TypeOf((*MockCloud)(nil).DescribeFileSystem), ctx, fileSystemId)
}

// DescribeMountTargets mocks base method.
func (m *MockCloud) DescribeMountTargets(ctx context.Context, fileSystemId, az string) (*cloud.MountTarget, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeMountTargets", ctx, fileSystemId, az)
	ret0, _ := ret[0].(*cloud.MountTarget)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeMountTargets indicates an expected call of DescribeMountTargets.
func (mr *MockCloudMockRecorder) DescribeMountTargets(ctx, fileSystemId, az interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeMountTargets", reflect.TypeOf((*MockCloud)(nil).DescribeMountTargets), ctx, fileSystemId, az)
}

// FindAccessPointByClientToken mocks base method.
func (m *MockCloud) FindAccessPointByClientToken(ctx context.Context, clientToken, fileSystemId string) (*cloud.AccessPoint, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindAccessPointByClientToken", ctx, clientToken, fileSystemId)
	ret0, _ := ret[0].(*cloud.AccessPoint)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindAccessPointByClientToken indicates an expected call of FindAccessPointByClientToken.
func (mr *MockCloudMockRecorder) FindAccessPointByClientToken(ctx, clientToken, fileSystemId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindAccessPointByClientToken", reflect.TypeOf((*MockCloud)(nil).FindAccessPointByClientToken), ctx, clientToken, fileSystemId)
}

// GetMetadata mocks base method.
func (m *MockCloud) GetMetadata() cloud.MetadataService {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMetadata")
	ret0, _ := ret[0].(cloud.MetadataService)
	return ret0
}

// GetMetadata indicates an expected call of GetMetadata.
func (mr *MockCloudMockRecorder) GetMetadata() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMetadata", reflect.TypeOf((*MockCloud)(nil).GetMetadata))
}

// ListAccessPoints mocks base method.
func (m *MockCloud) ListAccessPoints(ctx context.Context, fileSystemId string) ([]*cloud.AccessPoint, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAccessPoints", ctx, fileSystemId)
	ret0, _ := ret[0].([]*cloud.AccessPoint)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListAccessPoints indicates an expected call of ListAccessPoints.
func (mr *MockCloudMockRecorder) ListAccessPoints(ctx, fileSystemId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAccessPoints", reflect.TypeOf((*MockCloud)(nil).ListAccessPoints), ctx, fileSystemId)
}
