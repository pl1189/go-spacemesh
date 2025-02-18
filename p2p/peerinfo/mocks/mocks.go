// Code generated by MockGen. DO NOT EDIT.
// Source: ./peerinfo.go
//
// Generated by this command:
//
//	mockgen -typed -package=peerinfo -destination=./mocks/mocks.go -source=./peerinfo.go
//

// Package peerinfo is a generated GoMock package.
package peerinfo

import (
	reflect "reflect"

	peer "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
	peerinfo "github.com/spacemeshos/go-spacemesh/p2p/peerinfo"
	gomock "go.uber.org/mock/gomock"
)

// MockPeerInfo is a mock of PeerInfo interface.
type MockPeerInfo struct {
	ctrl     *gomock.Controller
	recorder *MockPeerInfoMockRecorder
}

// MockPeerInfoMockRecorder is the mock recorder for MockPeerInfo.
type MockPeerInfoMockRecorder struct {
	mock *MockPeerInfo
}

// NewMockPeerInfo creates a new mock instance.
func NewMockPeerInfo(ctrl *gomock.Controller) *MockPeerInfo {
	mock := &MockPeerInfo{ctrl: ctrl}
	mock.recorder = &MockPeerInfoMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPeerInfo) EXPECT() *MockPeerInfoMockRecorder {
	return m.recorder
}

// EnsurePeerInfo mocks base method.
func (m *MockPeerInfo) EnsurePeerInfo(p peer.ID) *peerinfo.Info {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnsurePeerInfo", p)
	ret0, _ := ret[0].(*peerinfo.Info)
	return ret0
}

// EnsurePeerInfo indicates an expected call of EnsurePeerInfo.
func (mr *MockPeerInfoMockRecorder) EnsurePeerInfo(p any) *MockPeerInfoEnsurePeerInfoCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnsurePeerInfo", reflect.TypeOf((*MockPeerInfo)(nil).EnsurePeerInfo), p)
	return &MockPeerInfoEnsurePeerInfoCall{Call: call}
}

// MockPeerInfoEnsurePeerInfoCall wrap *gomock.Call
type MockPeerInfoEnsurePeerInfoCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPeerInfoEnsurePeerInfoCall) Return(arg0 *peerinfo.Info) *MockPeerInfoEnsurePeerInfoCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPeerInfoEnsurePeerInfoCall) Do(f func(peer.ID) *peerinfo.Info) *MockPeerInfoEnsurePeerInfoCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPeerInfoEnsurePeerInfoCall) DoAndReturn(f func(peer.ID) *peerinfo.Info) *MockPeerInfoEnsurePeerInfoCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// EnsureProtoStats mocks base method.
func (m *MockPeerInfo) EnsureProtoStats(proto protocol.ID) *peerinfo.DataStats {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnsureProtoStats", proto)
	ret0, _ := ret[0].(*peerinfo.DataStats)
	return ret0
}

// EnsureProtoStats indicates an expected call of EnsureProtoStats.
func (mr *MockPeerInfoMockRecorder) EnsureProtoStats(proto any) *MockPeerInfoEnsureProtoStatsCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnsureProtoStats", reflect.TypeOf((*MockPeerInfo)(nil).EnsureProtoStats), proto)
	return &MockPeerInfoEnsureProtoStatsCall{Call: call}
}

// MockPeerInfoEnsureProtoStatsCall wrap *gomock.Call
type MockPeerInfoEnsureProtoStatsCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPeerInfoEnsureProtoStatsCall) Return(arg0 *peerinfo.DataStats) *MockPeerInfoEnsureProtoStatsCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPeerInfoEnsureProtoStatsCall) Do(f func(protocol.ID) *peerinfo.DataStats) *MockPeerInfoEnsureProtoStatsCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPeerInfoEnsureProtoStatsCall) DoAndReturn(f func(protocol.ID) *peerinfo.DataStats) *MockPeerInfoEnsureProtoStatsCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Protocols mocks base method.
func (m *MockPeerInfo) Protocols() []protocol.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Protocols")
	ret0, _ := ret[0].([]protocol.ID)
	return ret0
}

// Protocols indicates an expected call of Protocols.
func (mr *MockPeerInfoMockRecorder) Protocols() *MockPeerInfoProtocolsCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Protocols", reflect.TypeOf((*MockPeerInfo)(nil).Protocols))
	return &MockPeerInfoProtocolsCall{Call: call}
}

// MockPeerInfoProtocolsCall wrap *gomock.Call
type MockPeerInfoProtocolsCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPeerInfoProtocolsCall) Return(arg0 []protocol.ID) *MockPeerInfoProtocolsCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPeerInfoProtocolsCall) Do(f func() []protocol.ID) *MockPeerInfoProtocolsCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPeerInfoProtocolsCall) DoAndReturn(f func() []protocol.ID) *MockPeerInfoProtocolsCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// RecordReceived mocks base method.
func (m *MockPeerInfo) RecordReceived(n int64, proto protocol.ID, p peer.ID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RecordReceived", n, proto, p)
}

// RecordReceived indicates an expected call of RecordReceived.
func (mr *MockPeerInfoMockRecorder) RecordReceived(n, proto, p any) *MockPeerInfoRecordReceivedCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RecordReceived", reflect.TypeOf((*MockPeerInfo)(nil).RecordReceived), n, proto, p)
	return &MockPeerInfoRecordReceivedCall{Call: call}
}

// MockPeerInfoRecordReceivedCall wrap *gomock.Call
type MockPeerInfoRecordReceivedCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPeerInfoRecordReceivedCall) Return() *MockPeerInfoRecordReceivedCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPeerInfoRecordReceivedCall) Do(f func(int64, protocol.ID, peer.ID)) *MockPeerInfoRecordReceivedCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPeerInfoRecordReceivedCall) DoAndReturn(f func(int64, protocol.ID, peer.ID)) *MockPeerInfoRecordReceivedCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// RecordSent mocks base method.
func (m *MockPeerInfo) RecordSent(n int64, proto protocol.ID, p peer.ID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RecordSent", n, proto, p)
}

// RecordSent indicates an expected call of RecordSent.
func (mr *MockPeerInfoMockRecorder) RecordSent(n, proto, p any) *MockPeerInfoRecordSentCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RecordSent", reflect.TypeOf((*MockPeerInfo)(nil).RecordSent), n, proto, p)
	return &MockPeerInfoRecordSentCall{Call: call}
}

// MockPeerInfoRecordSentCall wrap *gomock.Call
type MockPeerInfoRecordSentCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockPeerInfoRecordSentCall) Return() *MockPeerInfoRecordSentCall {
	c.Call = c.Call.Return()
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockPeerInfoRecordSentCall) Do(f func(int64, protocol.ID, peer.ID)) *MockPeerInfoRecordSentCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockPeerInfoRecordSentCall) DoAndReturn(f func(int64, protocol.ID, peer.ID)) *MockPeerInfoRecordSentCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
