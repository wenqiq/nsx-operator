// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/vmware/vsphere-automation-sdk-go/services/nsxt/orgs/projects (interfaces: VpcsClient)

// Package mock_projects is a generated GoMock package.
package mock_projects

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	model "github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
)

// MockVpcsClient is a mock of VpcsClient interface.
type MockVpcsClient struct {
	ctrl     *gomock.Controller
	recorder *MockVpcsClientMockRecorder
}

// MockVpcsClientMockRecorder is the mock recorder for MockVpcsClient.
type MockVpcsClientMockRecorder struct {
	mock *MockVpcsClient
}

// NewMockVpcsClient creates a new mock instance.
func NewMockVpcsClient(ctrl *gomock.Controller) *MockVpcsClient {
	mock := &MockVpcsClient{ctrl: ctrl}
	mock.recorder = &MockVpcsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockVpcsClient) EXPECT() *MockVpcsClientMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockVpcsClient) Delete(arg0, arg1, arg2 string, is_recurse *bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockVpcsClientMockRecorder) Delete(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockVpcsClient)(nil).Delete), arg0, arg1, arg2)
}

// Get mocks base method.
func (m *MockVpcsClient) Get(arg0, arg1, arg2 string) (model.Vpc, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", arg0, arg1, arg2)
	ret0, _ := ret[0].(model.Vpc)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockVpcsClientMockRecorder) Get(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockVpcsClient)(nil).Get), arg0, arg1, arg2)
}

// List mocks base method.
func (m *MockVpcsClient) List(arg0, arg1 string, arg2 *string, arg3 *bool, arg4 *string, arg5 *int64, arg6 *bool, arg7 *string) (model.VpcListResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List", arg0, arg1, arg2, arg3, arg4, arg5, arg6, arg7)
	ret0, _ := ret[0].(model.VpcListResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// List indicates an expected call of List.
func (mr *MockVpcsClientMockRecorder) List(arg0, arg1, arg2, arg3, arg4, arg5, arg6, arg7 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "List", reflect.TypeOf((*MockVpcsClient)(nil).List), arg0, arg1, arg2, arg3, arg4, arg5, arg6, arg7)
}

// Patch mocks base method.
func (m *MockVpcsClient) Patch(arg0, arg1, arg2 string, arg3 model.Vpc) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Patch", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Patch indicates an expected call of Patch.
func (mr *MockVpcsClientMockRecorder) Patch(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Patch", reflect.TypeOf((*MockVpcsClient)(nil).Patch), arg0, arg1, arg2, arg3)
}

// Update mocks base method.
func (m *MockVpcsClient) Update(arg0, arg1, arg2 string, arg3 model.Vpc) (model.Vpc, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(model.Vpc)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Update indicates an expected call of Update.
func (mr *MockVpcsClientMockRecorder) Update(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockVpcsClient)(nil).Update), arg0, arg1, arg2, arg3)
}
