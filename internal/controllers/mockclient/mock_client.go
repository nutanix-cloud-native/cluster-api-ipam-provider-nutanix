// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client (interfaces: Client,ClusterClient,PrismClient,NetworkingClient)
//
// Generated by this command:
//
//	mockgen -copyright_file ../../hack/license-header.txt -typed -destination ./mockclient/mock_client.go -package mockclient github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client Client,ClusterClient,PrismClient,NetworkingClient
//

// Package mockclient is a generated GoMock package.
package mockclient

import (
	netip "net/netip"
	reflect "reflect"

	client "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
	config "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/common/v1/config"
	gomock "go.uber.org/mock/gomock"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// Cluster mocks base method.
func (m *MockClient) Cluster() client.ClusterClient {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Cluster")
	ret0, _ := ret[0].(client.ClusterClient)
	return ret0
}

// Cluster indicates an expected call of Cluster.
func (mr *MockClientMockRecorder) Cluster() *MockClientClusterCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Cluster", reflect.TypeOf((*MockClient)(nil).Cluster))
	return &MockClientClusterCall{Call: call}
}

// MockClientClusterCall wrap *gomock.Call
type MockClientClusterCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockClientClusterCall) Return(arg0 client.ClusterClient) *MockClientClusterCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockClientClusterCall) Do(f func() client.ClusterClient) *MockClientClusterCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockClientClusterCall) DoAndReturn(f func() client.ClusterClient) *MockClientClusterCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Networking mocks base method.
func (m *MockClient) Networking() client.NetworkingClient {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Networking")
	ret0, _ := ret[0].(client.NetworkingClient)
	return ret0
}

// Networking indicates an expected call of Networking.
func (mr *MockClientMockRecorder) Networking() *MockClientNetworkingCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Networking", reflect.TypeOf((*MockClient)(nil).Networking))
	return &MockClientNetworkingCall{Call: call}
}

// MockClientNetworkingCall wrap *gomock.Call
type MockClientNetworkingCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockClientNetworkingCall) Return(arg0 client.NetworkingClient) *MockClientNetworkingCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockClientNetworkingCall) Do(f func() client.NetworkingClient) *MockClientNetworkingCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockClientNetworkingCall) DoAndReturn(f func() client.NetworkingClient) *MockClientNetworkingCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Prism mocks base method.
func (m *MockClient) Prism() client.PrismClient {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Prism")
	ret0, _ := ret[0].(client.PrismClient)
	return ret0
}

// Prism indicates an expected call of Prism.
func (mr *MockClientMockRecorder) Prism() *MockClientPrismCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Prism", reflect.TypeOf((*MockClient)(nil).Prism))
	return &MockClientPrismCall{Call: call}
}

// MockClientPrismCall wrap *gomock.Call
type MockClientPrismCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockClientPrismCall) Return(arg0 client.PrismClient) *MockClientPrismCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockClientPrismCall) Do(f func() client.PrismClient) *MockClientPrismCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockClientPrismCall) DoAndReturn(f func() client.PrismClient) *MockClientPrismCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockClusterClient is a mock of ClusterClient interface.
type MockClusterClient struct {
	ctrl     *gomock.Controller
	recorder *MockClusterClientMockRecorder
}

// MockClusterClientMockRecorder is the mock recorder for MockClusterClient.
type MockClusterClientMockRecorder struct {
	mock *MockClusterClient
}

// NewMockClusterClient creates a new mock instance.
func NewMockClusterClient(ctrl *gomock.Controller) *MockClusterClient {
	mock := &MockClusterClient{ctrl: ctrl}
	mock.recorder = &MockClusterClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClusterClient) EXPECT() *MockClusterClientMockRecorder {
	return m.recorder
}

// GetCluster mocks base method.
func (m *MockClusterClient) GetCluster(arg0 string) (*client.Cluster, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCluster", arg0)
	ret0, _ := ret[0].(*client.Cluster)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCluster indicates an expected call of GetCluster.
func (mr *MockClusterClientMockRecorder) GetCluster(arg0 any) *MockClusterClientGetClusterCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCluster", reflect.TypeOf((*MockClusterClient)(nil).GetCluster), arg0)
	return &MockClusterClientGetClusterCall{Call: call}
}

// MockClusterClientGetClusterCall wrap *gomock.Call
type MockClusterClientGetClusterCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockClusterClientGetClusterCall) Return(arg0 *client.Cluster, arg1 error) *MockClusterClientGetClusterCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockClusterClientGetClusterCall) Do(f func(string) (*client.Cluster, error)) *MockClusterClientGetClusterCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockClusterClientGetClusterCall) DoAndReturn(f func(string) (*client.Cluster, error)) *MockClusterClientGetClusterCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockPrismClient is a mock of PrismClient interface.
type MockPrismClient struct {
	ctrl     *gomock.Controller
	recorder *MockPrismClientMockRecorder
}

// MockPrismClientMockRecorder is the mock recorder for MockPrismClient.
type MockPrismClientMockRecorder struct {
	mock *MockPrismClient
}

// NewMockPrismClient creates a new mock instance.
func NewMockPrismClient(ctrl *gomock.Controller) *MockPrismClient {
	mock := &MockPrismClient{ctrl: ctrl}
	mock.recorder = &MockPrismClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPrismClient) EXPECT() *MockPrismClientMockRecorder {
	return m.recorder
}

// GetTaskData mocks base method.
func (m *MockPrismClient) GetTaskData(arg0 string) ([]config.KVPair, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTaskData", arg0)
	ret0, _ := ret[0].([]config.KVPair)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTaskData indicates an expected call of GetTaskData.
func (mr *MockPrismClientMockRecorder) GetTaskData(arg0 any) *MockPrismClientGetTaskDataCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTaskData", reflect.TypeOf((*MockPrismClient)(nil).GetTaskData), arg0)
	return &MockPrismClientGetTaskDataCall{Call: call}
}

// MockPrismClientGetTaskDataCall wrap *gomock.Call
type MockPrismClientGetTaskDataCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPrismClientGetTaskDataCall) Return(arg0 []config.KVPair, arg1 error) *MockPrismClientGetTaskDataCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPrismClientGetTaskDataCall) Do(f func(string) ([]config.KVPair, error)) *MockPrismClientGetTaskDataCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPrismClientGetTaskDataCall) DoAndReturn(f func(string) ([]config.KVPair, error)) *MockPrismClientGetTaskDataCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockNetworkingClient is a mock of NetworkingClient interface.
type MockNetworkingClient struct {
	ctrl     *gomock.Controller
	recorder *MockNetworkingClientMockRecorder
}

// MockNetworkingClientMockRecorder is the mock recorder for MockNetworkingClient.
type MockNetworkingClientMockRecorder struct {
	mock *MockNetworkingClient
}

// NewMockNetworkingClient creates a new mock instance.
func NewMockNetworkingClient(ctrl *gomock.Controller) *MockNetworkingClient {
	mock := &MockNetworkingClient{ctrl: ctrl}
	mock.recorder = &MockNetworkingClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockNetworkingClient) EXPECT() *MockNetworkingClientMockRecorder {
	return m.recorder
}

// GetSubnet mocks base method.
func (m *MockNetworkingClient) GetSubnet(arg0 string, arg1 client.GetSubnetOpts) (*client.Subnet, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSubnet", arg0, arg1)
	ret0, _ := ret[0].(*client.Subnet)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSubnet indicates an expected call of GetSubnet.
func (mr *MockNetworkingClientMockRecorder) GetSubnet(arg0, arg1 any) *MockNetworkingClientGetSubnetCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSubnet", reflect.TypeOf((*MockNetworkingClient)(nil).GetSubnet), arg0, arg1)
	return &MockNetworkingClientGetSubnetCall{Call: call}
}

// MockNetworkingClientGetSubnetCall wrap *gomock.Call
type MockNetworkingClientGetSubnetCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockNetworkingClientGetSubnetCall) Return(arg0 *client.Subnet, arg1 error) *MockNetworkingClientGetSubnetCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockNetworkingClientGetSubnetCall) Do(f func(string, client.GetSubnetOpts) (*client.Subnet, error)) *MockNetworkingClientGetSubnetCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockNetworkingClientGetSubnetCall) DoAndReturn(f func(string, client.GetSubnetOpts) (*client.Subnet, error)) *MockNetworkingClientGetSubnetCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// ReserveIP mocks base method.
func (m *MockNetworkingClient) ReserveIP(arg0 client.IPReservationTypeFunc, arg1 string, arg2 client.ReserveIPOpts) ([]netip.Addr, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReserveIP", arg0, arg1, arg2)
	ret0, _ := ret[0].([]netip.Addr)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReserveIP indicates an expected call of ReserveIP.
func (mr *MockNetworkingClientMockRecorder) ReserveIP(arg0, arg1, arg2 any) *MockNetworkingClientReserveIPCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveIP", reflect.TypeOf((*MockNetworkingClient)(nil).ReserveIP), arg0, arg1, arg2)
	return &MockNetworkingClientReserveIPCall{Call: call}
}

// MockNetworkingClientReserveIPCall wrap *gomock.Call
type MockNetworkingClientReserveIPCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockNetworkingClientReserveIPCall) Return(arg0 []netip.Addr, arg1 error) *MockNetworkingClientReserveIPCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockNetworkingClientReserveIPCall) Do(f func(client.IPReservationTypeFunc, string, client.ReserveIPOpts) ([]netip.Addr, error)) *MockNetworkingClientReserveIPCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockNetworkingClientReserveIPCall) DoAndReturn(f func(client.IPReservationTypeFunc, string, client.ReserveIPOpts) ([]netip.Addr, error)) *MockNetworkingClientReserveIPCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// UnreserveIP mocks base method.
func (m *MockNetworkingClient) UnreserveIP(arg0 client.IPUnreservationTypeFunc, arg1 string, arg2 client.UnreserveIPOpts) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnreserveIP", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UnreserveIP indicates an expected call of UnreserveIP.
func (mr *MockNetworkingClientMockRecorder) UnreserveIP(arg0, arg1, arg2 any) *MockNetworkingClientUnreserveIPCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UnreserveIP", reflect.TypeOf((*MockNetworkingClient)(nil).UnreserveIP), arg0, arg1, arg2)
	return &MockNetworkingClientUnreserveIPCall{Call: call}
}

// MockNetworkingClientUnreserveIPCall wrap *gomock.Call
type MockNetworkingClientUnreserveIPCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockNetworkingClientUnreserveIPCall) Return(arg0 error) *MockNetworkingClientUnreserveIPCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockNetworkingClientUnreserveIPCall) Do(f func(client.IPUnreservationTypeFunc, string, client.UnreserveIPOpts) error) *MockNetworkingClientUnreserveIPCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockNetworkingClientUnreserveIPCall) DoAndReturn(f func(client.IPUnreservationTypeFunc, string, client.UnreserveIPOpts) error) *MockNetworkingClientUnreserveIPCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
