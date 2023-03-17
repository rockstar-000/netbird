package peer

// Listener is a callback type about the NetBird network connection state
type Listener interface {
	OnConnected()
	OnDisconnected()
	OnConnecting()
	OnPeersListChanged(int)
}
