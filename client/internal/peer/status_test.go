package peer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddPeer(t *testing.T) {
	key := "abc"
	status := NewRecorder()
	err := status.AddPeer(key)
	assert.NoError(t, err, "shouldn't return error")

	_, exists := status.peers[key]
	assert.True(t, exists, "value was found")

	err = status.AddPeer(key)

	assert.Error(t, err, "should return error on duplicate")
}

func TestGetPeer(t *testing.T) {
	key := "abc"
	status := NewRecorder()
	err := status.AddPeer(key)
	assert.NoError(t, err, "shouldn't return error")

	peerStatus, err := status.GetPeer(key)
	assert.NoError(t, err, "shouldn't return error on getting peer")

	assert.Equal(t, key, peerStatus.PubKey, "retrieved public key should match")

	_, err = status.GetPeer("non_existing_key")
	assert.Error(t, err, "should return error when peer doesn't exist")
}

func TestUpdatePeerState(t *testing.T) {
	key := "abc"
	ip := "10.10.10.10"
	status := NewRecorder()
	peerState := State{
		PubKey: key,
	}

	status.peers[key] = peerState

	peerState.IP = ip

	err := status.UpdatePeerState(peerState)
	assert.NoError(t, err, "shouldn't return error")

	state, exists := status.peers[key]
	assert.True(t, exists, "state should be found")
	assert.Equal(t, ip, state.IP, "ip should be equal")
}

func TestStatus_UpdatePeerFQDN(t *testing.T) {
	key := "abc"
	fqdn := "peer-a.netbird.local"
	status := NewRecorder()
	peerState := State{
		PubKey: key,
	}

	status.peers[key] = peerState

	err := status.UpdatePeerFQDN(key, fqdn)
	assert.NoError(t, err, "shouldn't return error")

	state, exists := status.peers[key]
	assert.True(t, exists, "state should be found")
	assert.Equal(t, fqdn, state.FQDN, "fqdn should be equal")
}

func TestGetPeerStateChangeNotifierLogic(t *testing.T) {
	key := "abc"
	ip := "10.10.10.10"
	status := NewRecorder()
	peerState := State{
		PubKey: key,
	}

	status.peers[key] = peerState

	ch := status.GetPeerStateChangeNotifier(key)
	assert.NotNil(t, ch, "channel shouldn't be nil")

	peerState.IP = ip

	err := status.UpdatePeerState(peerState)
	assert.NoError(t, err, "shouldn't return error")

	select {
	case <-ch:
	default:
		t.Errorf("channel wasn't closed after update")
	}
}

func TestRemovePeer(t *testing.T) {
	key := "abc"
	status := NewRecorder()
	peerState := State{
		PubKey: key,
	}

	status.peers[key] = peerState

	err := status.RemovePeer(key)
	assert.NoError(t, err, "shouldn't return error")

	_, exists := status.peers[key]
	assert.False(t, exists, "state value shouldn't be found")

	err = status.RemovePeer("not existing")
	assert.Error(t, err, "should return error when peer doesn't exist")
}

func TestUpdateLocalPeerState(t *testing.T) {
	localPeerState := LocalPeerState{
		IP:              "10.10.10.10",
		PubKey:          "abc",
		KernelInterface: false,
	}
	status := NewRecorder()

	status.UpdateLocalPeerState(localPeerState)

	assert.Equal(t, localPeerState, status.localPeer, "local peer status should be equal")
}

func TestCleanLocalPeerState(t *testing.T) {
	emptyLocalPeerState := LocalPeerState{}
	localPeerState := LocalPeerState{
		IP:              "10.10.10.10",
		PubKey:          "abc",
		KernelInterface: false,
	}
	status := NewRecorder()

	status.localPeer = localPeerState

	status.CleanLocalPeerState()

	assert.Equal(t, emptyLocalPeerState, status.localPeer, "local peer status should be empty")
}

func TestUpdateSignalState(t *testing.T) {
	url := "https://signal"
	var tests = []struct {
		name      string
		connected bool
		want      SignalState
	}{
		{"should mark as connected", true, SignalState{

			URL:       url,
			Connected: true,
		}},
		{"should mark as disconnected", false, SignalState{
			URL:       url,
			Connected: false,
		}},
	}

	status := NewRecorder()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.connected {
				status.MarkSignalConnected(url)
			} else {
				status.MarkSignalDisconnected(url)
			}
			assert.Equal(t, test.want, status.signal, "signal status should be equal")
		})
	}
}

func TestUpdateManagementState(t *testing.T) {
	url := "https://management"
	var tests = []struct {
		name      string
		connected bool
		want      ManagementState
	}{
		{"should mark as connected", true, ManagementState{

			URL:       url,
			Connected: true,
		}},
		{"should mark as disconnected", false, ManagementState{
			URL:       url,
			Connected: false,
		}},
	}

	status := NewRecorder()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.connected {
				status.MarkManagementConnected(url)
			} else {
				status.MarkManagementDisconnected(url)
			}
			assert.Equal(t, test.want, status.management, "signal status should be equal")
		})
	}
}

func TestGetFullStatus(t *testing.T) {
	key1 := "abc"
	key2 := "def"
	managementState := ManagementState{
		URL:       "https://signal",
		Connected: true,
	}
	signalState := SignalState{
		URL:       "https://signal",
		Connected: true,
	}
	peerState1 := State{
		PubKey: key1,
	}

	peerState2 := State{
		PubKey: key2,
	}

	status := NewRecorder()

	status.management = managementState
	status.signal = signalState
	status.peers[key1] = peerState1
	status.peers[key2] = peerState2

	fullStatus := status.GetFullStatus()

	assert.Equal(t, managementState, fullStatus.ManagementState, "management status should be equal")
	assert.Equal(t, signalState, fullStatus.SignalState, "signal status should be equal")
	assert.ElementsMatch(t, []State{peerState1, peerState2}, fullStatus.Peers, "peers states should match")
}
