package peer

import (
	"errors"
	"sync"
	"time"
)

// State contains the latest state of a peer
type State struct {
	IP                     string
	PubKey                 string
	FQDN                   string
	ConnStatus             ConnStatus
	ConnStatusUpdate       time.Time
	Relayed                bool
	Direct                 bool
	LocalIceCandidateType  string
	RemoteIceCandidateType string
}

// LocalPeerState contains the latest state of the local peer
type LocalPeerState struct {
	IP              string
	PubKey          string
	KernelInterface bool
	FQDN            string
}

// SignalState contains the latest state of a signal connection
type SignalState struct {
	URL       string
	Connected bool
}

// ManagementState contains the latest state of a management connection
type ManagementState struct {
	URL       string
	Connected bool
}

// FullStatus contains the full state held by the Status instance
type FullStatus struct {
	Peers           []State
	ManagementState ManagementState
	SignalState     SignalState
	LocalPeerState  LocalPeerState
}

// Status holds a state of peers, signal and management connections
type Status struct {
	mux             sync.Mutex
	peers           map[string]State
	changeNotify    map[string]chan struct{}
	signalState     bool
	managementState bool
	localPeer       LocalPeerState
	offlinePeers    []State
	mgmAddress      string
	signalAddress   string
	notifier        *notifier

	// To reduce the number of notification invocation this bool will be true when need to call the notification
	// Some Peer actions mostly used by in a batch when the network map has been synchronized. In these type of events
	// set to true this variable and at the end of the processing we will reset it by the FinishPeerListModifications()
	peerListChangedForNotification bool
}

// NewRecorder returns a new Status instance
func NewRecorder(mgmAddress string) *Status {
	return &Status{
		peers:        make(map[string]State),
		changeNotify: make(map[string]chan struct{}),
		offlinePeers: make([]State, 0),
		notifier:     newNotifier(),
		mgmAddress:   mgmAddress,
	}
}

// ReplaceOfflinePeers replaces
func (d *Status) ReplaceOfflinePeers(replacement []State) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.offlinePeers = make([]State, len(replacement))
	copy(d.offlinePeers, replacement)

	// todo we should set to true in case if the list changed only
	d.peerListChangedForNotification = true
}

// AddPeer adds peer to Daemon status map
func (d *Status) AddPeer(peerPubKey string, fqdn string) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	_, ok := d.peers[peerPubKey]
	if ok {
		return errors.New("peer already exist")
	}
	d.peers[peerPubKey] = State{
		PubKey:     peerPubKey,
		ConnStatus: StatusDisconnected,
		FQDN:       fqdn,
	}
	d.peerListChangedForNotification = true
	return nil
}

// GetPeer adds peer to Daemon status map
func (d *Status) GetPeer(peerPubKey string) (State, error) {
	d.mux.Lock()
	defer d.mux.Unlock()

	state, ok := d.peers[peerPubKey]
	if !ok {
		return State{}, errors.New("peer not found")
	}
	return state, nil
}

// RemovePeer removes peer from Daemon status map
func (d *Status) RemovePeer(peerPubKey string) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	_, ok := d.peers[peerPubKey]
	if !ok {
		return errors.New("no peer with to remove")
	}

	delete(d.peers, peerPubKey)
	d.peerListChangedForNotification = true
	return nil
}

// UpdatePeerState updates peer status
func (d *Status) UpdatePeerState(receivedState State) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	peerState, ok := d.peers[receivedState.PubKey]
	if !ok {
		return errors.New("peer doesn't exist")
	}

	if receivedState.IP != "" {
		peerState.IP = receivedState.IP
	}

	skipNotification := shouldSkipNotify(receivedState, peerState)

	if receivedState.ConnStatus != peerState.ConnStatus {
		peerState.ConnStatus = receivedState.ConnStatus
		peerState.ConnStatusUpdate = receivedState.ConnStatusUpdate
		peerState.Direct = receivedState.Direct
		peerState.Relayed = receivedState.Relayed
		peerState.LocalIceCandidateType = receivedState.LocalIceCandidateType
		peerState.RemoteIceCandidateType = receivedState.RemoteIceCandidateType
	}

	d.peers[receivedState.PubKey] = peerState

	if skipNotification {
		return nil
	}

	ch, found := d.changeNotify[receivedState.PubKey]
	if found && ch != nil {
		close(ch)
		d.changeNotify[receivedState.PubKey] = nil
	}

	d.notifyPeerListChanged()
	return nil
}

func shouldSkipNotify(received, curr State) bool {
	switch {
	case received.ConnStatus == StatusConnecting:
		return true
	case received.ConnStatus == StatusDisconnected && curr.ConnStatus == StatusConnecting:
		return true
	case received.ConnStatus == StatusDisconnected && curr.ConnStatus == StatusDisconnected:
		return curr.IP != ""
	default:
		return false
	}
}

// UpdatePeerFQDN update peer's state fqdn only
func (d *Status) UpdatePeerFQDN(peerPubKey, fqdn string) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	peerState, ok := d.peers[peerPubKey]
	if !ok {
		return errors.New("peer doesn't exist")
	}

	peerState.FQDN = fqdn
	d.peers[peerPubKey] = peerState

	return nil
}

// FinishPeerListModifications this event invoke the notification
func (d *Status) FinishPeerListModifications() {
	d.mux.Lock()

	if !d.peerListChangedForNotification {
		d.mux.Unlock()
		return
	}
	d.peerListChangedForNotification = false
	d.mux.Unlock()

	d.notifyPeerListChanged()
}

// GetPeerStateChangeNotifier returns a change notifier channel for a peer
func (d *Status) GetPeerStateChangeNotifier(peer string) <-chan struct{} {
	d.mux.Lock()
	defer d.mux.Unlock()
	ch, found := d.changeNotify[peer]
	if !found || ch == nil {
		ch = make(chan struct{})
		d.changeNotify[peer] = ch
	}
	return ch
}

// UpdateLocalPeerState updates local peer status
func (d *Status) UpdateLocalPeerState(localPeerState LocalPeerState) {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.localPeer = localPeerState
	d.notifyAddressChanged()
}

// CleanLocalPeerState cleans local peer status
func (d *Status) CleanLocalPeerState() {
	d.mux.Lock()
	defer d.mux.Unlock()

	d.localPeer = LocalPeerState{}
	d.notifyAddressChanged()
}

// MarkManagementDisconnected sets ManagementState to disconnected
func (d *Status) MarkManagementDisconnected() {
	d.mux.Lock()
	defer d.mux.Unlock()
	defer d.onConnectionChanged()

	d.managementState = false
}

// MarkManagementConnected sets ManagementState to connected
func (d *Status) MarkManagementConnected() {
	d.mux.Lock()
	defer d.mux.Unlock()
	defer d.onConnectionChanged()

	d.managementState = true
}

// UpdateSignalAddress update the address of the signal server
func (d *Status) UpdateSignalAddress(signalURL string) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.signalAddress = signalURL
}

// UpdateManagementAddress update the address of the management server
func (d *Status) UpdateManagementAddress(mgmAddress string) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.mgmAddress = mgmAddress
}

// MarkSignalDisconnected sets SignalState to disconnected
func (d *Status) MarkSignalDisconnected() {
	d.mux.Lock()
	defer d.mux.Unlock()
	defer d.onConnectionChanged()

	d.signalState = false
}

// MarkSignalConnected sets SignalState to connected
func (d *Status) MarkSignalConnected() {
	d.mux.Lock()
	defer d.mux.Unlock()
	defer d.onConnectionChanged()

	d.signalState = true
}

// GetFullStatus gets full status
func (d *Status) GetFullStatus() FullStatus {
	d.mux.Lock()
	defer d.mux.Unlock()

	fullStatus := FullStatus{
		ManagementState: ManagementState{
			d.mgmAddress,
			d.managementState,
		},
		SignalState: SignalState{
			d.signalAddress,
			d.signalState,
		},
		LocalPeerState: d.localPeer,
	}

	for _, status := range d.peers {
		fullStatus.Peers = append(fullStatus.Peers, status)
	}

	fullStatus.Peers = append(fullStatus.Peers, d.offlinePeers...)

	return fullStatus
}

// ClientStart will notify all listeners about the new service state
func (d *Status) ClientStart() {
	d.notifier.clientStart()
}

// ClientStop will notify all listeners about the new service state
func (d *Status) ClientStop() {
	d.notifier.clientStop()
}

// ClientTeardown will notify all listeners about the service is under teardown
func (d *Status) ClientTeardown() {
	d.notifier.clientTearDown()
}

// SetConnectionListener set a listener to the notifier
func (d *Status) SetConnectionListener(listener Listener) {
	d.notifier.setListener(listener)
}

// RemoveConnectionListener remove the listener from the notifier
func (d *Status) RemoveConnectionListener() {
	d.notifier.removeListener()
}

func (d *Status) onConnectionChanged() {
	d.notifier.updateServerStates(d.managementState, d.signalState)
}

func (d *Status) notifyPeerListChanged() {
	d.notifier.peerListChanged(d.numOfPeers())
}

func (d *Status) notifyAddressChanged() {
	d.notifier.localAddressChanged(d.localPeer.FQDN, d.localPeer.IP)
}

func (d *Status) numOfPeers() int {
	return len(d.peers) + len(d.offlinePeers)
}
