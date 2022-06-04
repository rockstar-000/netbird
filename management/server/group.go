package server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Group of the peers for ACL
type Group struct {
	// ID of the group
	ID string

	// Name visible in the UI
	Name string

	// Peers list of the group
	Peers []string
}

func (g *Group) Copy() *Group {
	return &Group{
		ID:    g.ID,
		Name:  g.Name,
		Peers: g.Peers[:],
	}
}

// GetGroup object of the peers
func (am *DefaultAccountManager) GetGroup(accountID, groupID string) (*Group, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	group, ok := account.Groups[groupID]
	if ok {
		return group, nil
	}

	return nil, status.Errorf(codes.NotFound, "group with ID %s not found", groupID)
}

// SaveGroup object of the peers
func (am *DefaultAccountManager) SaveGroup(accountID string, group *Group) error {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return status.Errorf(codes.NotFound, "account not found")
	}

	account.Groups[group.ID] = group

	account.Network.IncSerial()
	if err = am.Store.SaveAccount(account); err != nil {
		return err
	}

	return am.updateAccountPeers(account)
}

// DeleteGroup object of the peers
func (am *DefaultAccountManager) DeleteGroup(accountID, groupID string) error {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return status.Errorf(codes.NotFound, "account not found")
	}

	delete(account.Groups, groupID)

	account.Network.IncSerial()
	if err = am.Store.SaveAccount(account); err != nil {
		return err
	}

	return am.updateAccountPeers(account)
}

// ListGroups objects of the peers
func (am *DefaultAccountManager) ListGroups(accountID string) ([]*Group, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	groups := make([]*Group, 0, len(account.Groups))
	for _, item := range account.Groups {
		groups = append(groups, item)
	}

	return groups, nil
}

// GroupAddPeer appends peer to the group
func (am *DefaultAccountManager) GroupAddPeer(accountID, groupID, peerKey string) error {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return status.Errorf(codes.NotFound, "account not found")
	}

	group, ok := account.Groups[groupID]
	if !ok {
		return status.Errorf(codes.NotFound, "group with ID %s not found", groupID)
	}

	add := true
	for _, itemID := range group.Peers {
		if itemID == peerKey {
			add = false
			break
		}
	}
	if add {
		group.Peers = append(group.Peers, peerKey)
	}

	account.Network.IncSerial()
	if err = am.Store.SaveAccount(account); err != nil {
		return err
	}

	return am.updateAccountPeers(account)
}

// GroupDeletePeer removes peer from the group
func (am *DefaultAccountManager) GroupDeletePeer(accountID, groupID, peerKey string) error {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return status.Errorf(codes.NotFound, "account not found")
	}

	group, ok := account.Groups[groupID]
	if !ok {
		return status.Errorf(codes.NotFound, "group with ID %s not found", groupID)
	}

	account.Network.IncSerial()
	for i, itemID := range group.Peers {
		if itemID == peerKey {
			group.Peers = append(group.Peers[:i], group.Peers[i+1:]...)
			if err := am.Store.SaveAccount(account); err != nil {
				return status.Errorf(codes.Internal, "can't save account")
			}
		}
	}

	return am.updateAccountPeers(account)
}

// GroupListPeers returns list of the peers from the group
func (am *DefaultAccountManager) GroupListPeers(accountID, groupID string) ([]*Peer, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	group, ok := account.Groups[groupID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "group with ID %s not found", groupID)
	}

	peers := make([]*Peer, 0, len(account.Groups))
	for _, peerID := range group.Peers {
		p, ok := account.Peers[peerID]
		if ok {
			peers = append(peers, p)
		}
	}

	return peers, nil
}
