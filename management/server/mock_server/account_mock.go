package mock_server

import (
	"github.com/netbirdio/netbird/management/server"
	"github.com/netbirdio/netbird/management/server/jwtclaims"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type MockAccountManager struct {
	GetOrCreateAccountByUserFunc          func(userId, domain string) (*server.Account, error)
	GetAccountByUserFunc                  func(userId string) (*server.Account, error)
	AddSetupKeyFunc                       func(accountId string, keyName string, keyType server.SetupKeyType, expiresIn time.Duration) (*server.SetupKey, error)
	RevokeSetupKeyFunc                    func(accountId string, keyId string) (*server.SetupKey, error)
	RenameSetupKeyFunc                    func(accountId string, keyId string, newName string) (*server.SetupKey, error)
	GetAccountByIdFunc                    func(accountId string) (*server.Account, error)
	GetAccountByUserOrAccountIdFunc       func(userId, accountId, domain string) (*server.Account, error)
	GetAccountWithAuthorizationClaimsFunc func(claims jwtclaims.AuthorizationClaims) (*server.Account, error)
	IsUserAdminFunc                       func(claims jwtclaims.AuthorizationClaims) (bool, error)
	AccountExistsFunc                     func(accountId string) (*bool, error)
	GetPeerFunc                           func(peerKey string) (*server.Peer, error)
	MarkPeerConnectedFunc                 func(peerKey string, connected bool) error
	RenamePeerFunc                        func(accountId string, peerKey string, newName string) (*server.Peer, error)
	DeletePeerFunc                        func(accountId string, peerKey string) (*server.Peer, error)
	GetPeerByIPFunc                       func(accountId string, peerIP string) (*server.Peer, error)
	GetNetworkMapFunc                     func(peerKey string) (*server.NetworkMap, error)
	AddPeerFunc                           func(setupKey string, userId string, peer *server.Peer) (*server.Peer, error)
	GetGroupFunc                          func(accountID, groupID string) (*server.Group, error)
	SaveGroupFunc                         func(accountID string, group *server.Group) error
	UpdateGroupFunc                       func(accountID string, groupID string, operations []server.GroupUpdateOperation) (*server.Group, error)
	DeleteGroupFunc                       func(accountID, groupID string) error
	ListGroupsFunc                        func(accountID string) ([]*server.Group, error)
	GroupAddPeerFunc                      func(accountID, groupID, peerKey string) error
	GroupDeletePeerFunc                   func(accountID, groupID, peerKey string) error
	GroupListPeersFunc                    func(accountID, groupID string) ([]*server.Peer, error)
	GetRuleFunc                           func(accountID, ruleID string) (*server.Rule, error)
	SaveRuleFunc                          func(accountID string, rule *server.Rule) error
	UpdateRuleFunc                        func(accountID string, ruleID string, operations []server.RuleUpdateOperation) (*server.Rule, error)
	DeleteRuleFunc                        func(accountID, ruleID string) error
	ListRulesFunc                         func(accountID string) ([]*server.Rule, error)
	GetUsersFromAccountFunc               func(accountID string) ([]*server.UserInfo, error)
	UpdatePeerMetaFunc                    func(peerKey string, meta server.PeerSystemMeta) error
}

// GetUsersFromAccount mock implementation of GetUsersFromAccount from server.AccountManager interface
func (am *MockAccountManager) GetUsersFromAccount(accountID string) ([]*server.UserInfo, error) {
	if am.GetUsersFromAccountFunc != nil {
		return am.GetUsersFromAccountFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetUsersFromAccount not implemented")
}

// GetOrCreateAccountByUser mock implementation of GetOrCreateAccountByUser from server.AccountManager interface
func (am *MockAccountManager) GetOrCreateAccountByUser(
	userId, domain string,
) (*server.Account, error) {
	if am.GetOrCreateAccountByUserFunc != nil {
		return am.GetOrCreateAccountByUserFunc(userId, domain)
	}
	return nil, status.Errorf(
		codes.Unimplemented,
		"method GetOrCreateAccountByUser not implemented",
	)
}

// GetAccountByUser mock implementation of GetAccountByUser from server.AccountManager interface
func (am *MockAccountManager) GetAccountByUser(userId string) (*server.Account, error) {
	if am.GetAccountByUserFunc != nil {
		return am.GetAccountByUserFunc(userId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountByUser not implemented")
}

// AddSetupKey mock implementation of AddSetupKey from server.AccountManager interface
func (am *MockAccountManager) AddSetupKey(
	accountId string,
	keyName string,
	keyType server.SetupKeyType,
	expiresIn time.Duration,
) (*server.SetupKey, error) {
	if am.AddSetupKeyFunc != nil {
		return am.AddSetupKeyFunc(accountId, keyName, keyType, expiresIn)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AddSetupKey not implemented")
}

// RevokeSetupKey mock implementation of RevokeSetupKey from server.AccountManager interface
func (am *MockAccountManager) RevokeSetupKey(
	accountId string,
	keyId string,
) (*server.SetupKey, error) {
	if am.RevokeSetupKeyFunc != nil {
		return am.RevokeSetupKeyFunc(accountId, keyId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method RevokeSetupKey not implemented")
}

// RenameSetupKey mock implementation of RenameSetupKey from server.AccountManager interface
func (am *MockAccountManager) RenameSetupKey(
	accountId string,
	keyId string,
	newName string,
) (*server.SetupKey, error) {
	if am.RenameSetupKeyFunc != nil {
		return am.RenameSetupKeyFunc(accountId, keyId, newName)
	}
	return nil, status.Errorf(codes.Unimplemented, "method RenameSetupKey not implemented")
}

// GetAccountById mock implementation of GetAccountById from server.AccountManager interface
func (am *MockAccountManager) GetAccountById(accountId string) (*server.Account, error) {
	if am.GetAccountByIdFunc != nil {
		return am.GetAccountByIdFunc(accountId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountById not implemented")
}

// GetAccountByUserOrAccountId mock implementation of GetAccountByUserOrAccountId from server.AccountManager interface
func (am *MockAccountManager) GetAccountByUserOrAccountId(
	userId, accountId, domain string,
) (*server.Account, error) {
	if am.GetAccountByUserOrAccountIdFunc != nil {
		return am.GetAccountByUserOrAccountIdFunc(userId, accountId, domain)
	}
	return nil, status.Errorf(
		codes.Unimplemented,
		"method GetAccountByUserOrAccountId not implemented",
	)
}

// GetAccountWithAuthorizationClaims mock implementation of GetAccountWithAuthorizationClaims from server.AccountManager interface
func (am *MockAccountManager) GetAccountWithAuthorizationClaims(
	claims jwtclaims.AuthorizationClaims,
) (*server.Account, error) {
	if am.GetAccountWithAuthorizationClaimsFunc != nil {
		return am.GetAccountWithAuthorizationClaimsFunc(claims)
	}
	return nil, status.Errorf(
		codes.Unimplemented,
		"method GetAccountWithAuthorizationClaims not implemented",
	)
}

// AccountExists mock implementation of AccountExists from server.AccountManager interface
func (am *MockAccountManager) AccountExists(accountId string) (*bool, error) {
	if am.AccountExistsFunc != nil {
		return am.AccountExistsFunc(accountId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AccountExists not implemented")
}

// GetPeer mock implementation of GetPeer from server.AccountManager interface
func (am *MockAccountManager) GetPeer(peerKey string) (*server.Peer, error) {
	if am.GetPeerFunc != nil {
		return am.GetPeerFunc(peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetPeer not implemented")
}

// MarkPeerConnected mock implementation of MarkPeerConnected from server.AccountManager interface
func (am *MockAccountManager) MarkPeerConnected(peerKey string, connected bool) error {
	if am.MarkPeerConnectedFunc != nil {
		return am.MarkPeerConnectedFunc(peerKey, connected)
	}
	return status.Errorf(codes.Unimplemented, "method MarkPeerConnected not implemented")
}

// RenamePeer mock implementation of RenamePeer from server.AccountManager interface
func (am *MockAccountManager) RenamePeer(
	accountId string,
	peerKey string,
	newName string,
) (*server.Peer, error) {
	if am.RenamePeerFunc != nil {
		return am.RenamePeerFunc(accountId, peerKey, newName)
	}
	return nil, status.Errorf(codes.Unimplemented, "method RenamePeer not implemented")
}

// DeletePeer mock implementation of DeletePeer from server.AccountManager interface
func (am *MockAccountManager) DeletePeer(accountId string, peerKey string) (*server.Peer, error) {
	if am.DeletePeerFunc != nil {
		return am.DeletePeerFunc(accountId, peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method DeletePeer not implemented")
}

// GetPeerByIP mock implementation of GetPeerByIP from server.AccountManager interface
func (am *MockAccountManager) GetPeerByIP(accountId string, peerIP string) (*server.Peer, error) {
	if am.GetPeerByIPFunc != nil {
		return am.GetPeerByIPFunc(accountId, peerIP)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetPeerByIP not implemented")
}

// GetNetworkMap mock implementation of GetNetworkMap from server.AccountManager interface
func (am *MockAccountManager) GetNetworkMap(peerKey string) (*server.NetworkMap, error) {
	if am.GetNetworkMapFunc != nil {
		return am.GetNetworkMapFunc(peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetNetworkMap not implemented")
}

// AddPeer mock implementation of AddPeer from server.AccountManager interface
func (am *MockAccountManager) AddPeer(
	setupKey string,
	userId string,
	peer *server.Peer,
) (*server.Peer, error) {
	if am.AddPeerFunc != nil {
		return am.AddPeerFunc(setupKey, userId, peer)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AddPeer not implemented")
}

// GetGroup mock implementation of GetGroup from server.AccountManager interface
func (am *MockAccountManager) GetGroup(accountID, groupID string) (*server.Group, error) {
	if am.GetGroupFunc != nil {
		return am.GetGroupFunc(accountID, groupID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetGroup not implemented")
}

// SaveGroup mock implementation of SaveGroup from server.AccountManager interface
func (am *MockAccountManager) SaveGroup(accountID string, group *server.Group) error {
	if am.SaveGroupFunc != nil {
		return am.SaveGroupFunc(accountID, group)
	}
	return status.Errorf(codes.Unimplemented, "method SaveGroup not implemented")
}

// UpdateGroup mock implementation of UpdateGroup from server.AccountManager interface
func (am *MockAccountManager) UpdateGroup(accountID string, groupID string, operations []server.GroupUpdateOperation) (*server.Group, error) {
	if am.UpdateGroupFunc != nil {
		return am.UpdateGroupFunc(accountID, groupID, operations)
	}
	return nil, status.Errorf(codes.Unimplemented, "method UpdateGroup not implemented")
}

// DeleteGroup mock implementation of DeleteGroup from server.AccountManager interface
func (am *MockAccountManager) DeleteGroup(accountID, groupID string) error {
	if am.DeleteGroupFunc != nil {
		return am.DeleteGroupFunc(accountID, groupID)
	}
	return status.Errorf(codes.Unimplemented, "method DeleteGroup not implemented")
}

// ListGroups mock implementation of ListGroups from server.AccountManager interface
func (am *MockAccountManager) ListGroups(accountID string) ([]*server.Group, error) {
	if am.ListGroupsFunc != nil {
		return am.ListGroupsFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method ListGroups not implemented")
}

// GroupAddPeer mock implementation of GroupAddPeer from server.AccountManager interface
func (am *MockAccountManager) GroupAddPeer(accountID, groupID, peerKey string) error {
	if am.GroupAddPeerFunc != nil {
		return am.GroupAddPeerFunc(accountID, groupID, peerKey)
	}
	return status.Errorf(codes.Unimplemented, "method GroupAddPeer not implemented")
}

// GroupDeletePeer mock implementation of GroupDeletePeer from server.AccountManager interface
func (am *MockAccountManager) GroupDeletePeer(accountID, groupID, peerKey string) error {
	if am.GroupDeletePeerFunc != nil {
		return am.GroupDeletePeerFunc(accountID, groupID, peerKey)
	}
	return status.Errorf(codes.Unimplemented, "method GroupDeletePeer not implemented")
}

// GroupListPeers mock implementation of GroupListPeers from server.AccountManager interface
func (am *MockAccountManager) GroupListPeers(accountID, groupID string) ([]*server.Peer, error) {
	if am.GroupListPeersFunc != nil {
		return am.GroupListPeersFunc(accountID, groupID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GroupListPeers not implemented")
}

// GetRule mock implementation of GetRule from server.AccountManager interface
func (am *MockAccountManager) GetRule(accountID, ruleID string) (*server.Rule, error) {
	if am.GetRuleFunc != nil {
		return am.GetRuleFunc(accountID, ruleID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetRule not implemented")
}

// SaveRule mock implementation of SaveRule from server.AccountManager interface
func (am *MockAccountManager) SaveRule(accountID string, rule *server.Rule) error {
	if am.SaveRuleFunc != nil {
		return am.SaveRuleFunc(accountID, rule)
	}
	return status.Errorf(codes.Unimplemented, "method SaveRule not implemented")
}

// UpdateRule mock implementation of UpdateRule from server.AccountManager interface
func (am *MockAccountManager) UpdateRule(accountID string, ruleID string, operations []server.RuleUpdateOperation) (*server.Rule, error) {
	if am.UpdateRuleFunc != nil {
		return am.UpdateRuleFunc(accountID, ruleID, operations)
	}
	return nil, status.Errorf(codes.Unimplemented, "method UpdateRule not implemented")
}

// DeleteRule mock implementation of DeleteRule from server.AccountManager interface
func (am *MockAccountManager) DeleteRule(accountID, ruleID string) error {
	if am.DeleteRuleFunc != nil {
		return am.DeleteRuleFunc(accountID, ruleID)
	}
	return status.Errorf(codes.Unimplemented, "method DeleteRule not implemented")
}

// ListRules mock implementation of ListRules from server.AccountManager interface
func (am *MockAccountManager) ListRules(accountID string) ([]*server.Rule, error) {
	if am.ListRulesFunc != nil {
		return am.ListRulesFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method ListRules not implemented")
}

// UpdatePeerMeta mock implementation of UpdatePeerMeta from server.AccountManager interface
func (am *MockAccountManager) UpdatePeerMeta(peerKey string, meta server.PeerSystemMeta) error {
	if am.UpdatePeerMetaFunc != nil {
		return am.UpdatePeerMetaFunc(peerKey, meta)
	}
	return status.Errorf(codes.Unimplemented, "method UpdatePeerMetaFunc not implemented")
}

// IsUserAdmin mock implementation of IsUserAdmin from server.AccountManager interface
func (am *MockAccountManager) IsUserAdmin(claims jwtclaims.AuthorizationClaims) (bool, error) {
	if am.IsUserAdminFunc != nil {
		return am.IsUserAdminFunc(claims)
	}
	return false, status.Errorf(codes.Unimplemented, "method IsUserAdmin not implemented")
}
