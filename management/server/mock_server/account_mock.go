package mock_server

import (
	"github.com/netbirdio/netbird/management/server"
	"github.com/netbirdio/netbird/management/server/jwtclaims"
	"github.com/netbirdio/netbird/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MockAccountManager struct {
	GetOrCreateAccountByUserFunc          func(userId, domain string) (*server.Account, error)
	GetAccountByUserFunc                  func(userId string) (*server.Account, error)
	AddSetupKeyFunc                       func(accountId string, keyName string, keyType server.SetupKeyType, expiresIn *util.Duration) (*server.SetupKey, error)
	RevokeSetupKeyFunc                    func(accountId string, keyId string) (*server.SetupKey, error)
	RenameSetupKeyFunc                    func(accountId string, keyId string, newName string) (*server.SetupKey, error)
	GetAccountByIdFunc                    func(accountId string) (*server.Account, error)
	GetAccountByUserOrAccountIdFunc       func(userId, accountId, domain string) (*server.Account, error)
	GetAccountWithAuthorizationClaimsFunc func(claims jwtclaims.AuthorizationClaims) (*server.Account, error)
	IsUserAdminFunc                       func(claims jwtclaims.AuthorizationClaims) (bool, error)
	AccountExistsFunc                     func(accountId string) (*bool, error)
	AddAccountFunc                        func(accountId, userId, domain string) (*server.Account, error)
	GetPeerFunc                           func(peerKey string) (*server.Peer, error)
	MarkPeerConnectedFunc                 func(peerKey string, connected bool) error
	RenamePeerFunc                        func(accountId string, peerKey string, newName string) (*server.Peer, error)
	DeletePeerFunc                        func(accountId string, peerKey string) (*server.Peer, error)
	GetPeerByIPFunc                       func(accountId string, peerIP string) (*server.Peer, error)
	GetNetworkMapFunc                     func(peerKey string) (*server.NetworkMap, error)
	AddPeerFunc                           func(setupKey string, userId string, peer *server.Peer) (*server.Peer, error)
	GetGroupFunc                          func(accountID, groupID string) (*server.Group, error)
	SaveGroupFunc                         func(accountID string, group *server.Group) error
	DeleteGroupFunc                       func(accountID, groupID string) error
	ListGroupsFunc                        func(accountID string) ([]*server.Group, error)
	GroupAddPeerFunc                      func(accountID, groupID, peerKey string) error
	GroupDeletePeerFunc                   func(accountID, groupID, peerKey string) error
	GroupListPeersFunc                    func(accountID, groupID string) ([]*server.Peer, error)
	GetRuleFunc                           func(accountID, ruleID string) (*server.Rule, error)
	SaveRuleFunc                          func(accountID string, rule *server.Rule) error
	DeleteRuleFunc                        func(accountID, ruleID string) error
	ListRulesFunc                         func(accountID string) ([]*server.Rule, error)
	GetUsersFromAccountFunc               func(accountID string) ([]*server.UserInfo, error)
	UpdatePeerMetaFunc                    func(peerKey string, meta server.PeerSystemMeta) error
}

func (am *MockAccountManager) GetUsersFromAccount(accountID string) ([]*server.UserInfo, error) {
	if am.GetUsersFromAccountFunc != nil {
		return am.GetUsersFromAccountFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetUsersFromAccount not implemented")
}

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

func (am *MockAccountManager) GetAccountByUser(userId string) (*server.Account, error) {
	if am.GetAccountByUserFunc != nil {
		return am.GetAccountByUserFunc(userId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountByUser not implemented")
}

func (am *MockAccountManager) AddSetupKey(
	accountId string,
	keyName string,
	keyType server.SetupKeyType,
	expiresIn *util.Duration,
) (*server.SetupKey, error) {
	if am.AddSetupKeyFunc != nil {
		return am.AddSetupKeyFunc(accountId, keyName, keyType, expiresIn)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AddSetupKey not implemented")
}

func (am *MockAccountManager) RevokeSetupKey(
	accountId string,
	keyId string,
) (*server.SetupKey, error) {
	if am.RevokeSetupKeyFunc != nil {
		return am.RevokeSetupKeyFunc(accountId, keyId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method RevokeSetupKey not implemented")
}

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

func (am *MockAccountManager) GetAccountById(accountId string) (*server.Account, error) {
	if am.GetAccountByIdFunc != nil {
		return am.GetAccountByIdFunc(accountId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetAccountById not implemented")
}

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

func (am *MockAccountManager) AccountExists(accountId string) (*bool, error) {
	if am.AccountExistsFunc != nil {
		return am.AccountExistsFunc(accountId)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AccountExists not implemented")
}

func (am *MockAccountManager) AddAccount(
	accountId, userId, domain string,
) (*server.Account, error) {
	if am.AddAccountFunc != nil {
		return am.AddAccountFunc(accountId, userId, domain)
	}
	return nil, status.Errorf(codes.Unimplemented, "method AddAccount not implemented")
}

func (am *MockAccountManager) GetPeer(peerKey string) (*server.Peer, error) {
	if am.GetPeerFunc != nil {
		return am.GetPeerFunc(peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetPeer not implemented")
}

func (am *MockAccountManager) MarkPeerConnected(peerKey string, connected bool) error {
	if am.MarkPeerConnectedFunc != nil {
		return am.MarkPeerConnectedFunc(peerKey, connected)
	}
	return status.Errorf(codes.Unimplemented, "method MarkPeerConnected not implemented")
}

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

func (am *MockAccountManager) DeletePeer(accountId string, peerKey string) (*server.Peer, error) {
	if am.DeletePeerFunc != nil {
		return am.DeletePeerFunc(accountId, peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method DeletePeer not implemented")
}

func (am *MockAccountManager) GetPeerByIP(accountId string, peerIP string) (*server.Peer, error) {
	if am.GetPeerByIPFunc != nil {
		return am.GetPeerByIPFunc(accountId, peerIP)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetPeerByIP not implemented")
}

func (am *MockAccountManager) GetNetworkMap(peerKey string) (*server.NetworkMap, error) {
	if am.GetNetworkMapFunc != nil {
		return am.GetNetworkMapFunc(peerKey)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetNetworkMap not implemented")
}

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

func (am *MockAccountManager) GetGroup(accountID, groupID string) (*server.Group, error) {
	if am.GetGroupFunc != nil {
		return am.GetGroupFunc(accountID, groupID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetGroup not implemented")
}

func (am *MockAccountManager) SaveGroup(accountID string, group *server.Group) error {
	if am.SaveGroupFunc != nil {
		return am.SaveGroupFunc(accountID, group)
	}
	return status.Errorf(codes.Unimplemented, "method SaveGroup not implemented")
}

func (am *MockAccountManager) DeleteGroup(accountID, groupID string) error {
	if am.DeleteGroupFunc != nil {
		return am.DeleteGroupFunc(accountID, groupID)
	}
	return status.Errorf(codes.Unimplemented, "method DeleteGroup not implemented")
}

func (am *MockAccountManager) ListGroups(accountID string) ([]*server.Group, error) {
	if am.ListGroupsFunc != nil {
		return am.ListGroupsFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method ListGroups not implemented")
}

func (am *MockAccountManager) GroupAddPeer(accountID, groupID, peerKey string) error {
	if am.GroupAddPeerFunc != nil {
		return am.GroupAddPeerFunc(accountID, groupID, peerKey)
	}
	return status.Errorf(codes.Unimplemented, "method GroupAddPeer not implemented")
}

func (am *MockAccountManager) GroupDeletePeer(accountID, groupID, peerKey string) error {
	if am.GroupDeletePeerFunc != nil {
		return am.GroupDeletePeerFunc(accountID, groupID, peerKey)
	}
	return status.Errorf(codes.Unimplemented, "method GroupDeletePeer not implemented")
}

func (am *MockAccountManager) GroupListPeers(accountID, groupID string) ([]*server.Peer, error) {
	if am.GroupListPeersFunc != nil {
		return am.GroupListPeersFunc(accountID, groupID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GroupListPeers not implemented")
}

func (am *MockAccountManager) GetRule(accountID, ruleID string) (*server.Rule, error) {
	if am.GetRuleFunc != nil {
		return am.GetRuleFunc(accountID, ruleID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method GetRule not implemented")
}

func (am *MockAccountManager) SaveRule(accountID string, rule *server.Rule) error {
	if am.SaveRuleFunc != nil {
		return am.SaveRuleFunc(accountID, rule)
	}
	return status.Errorf(codes.Unimplemented, "method SaveRule not implemented")
}

func (am *MockAccountManager) DeleteRule(accountID, ruleID string) error {
	if am.DeleteRuleFunc != nil {
		return am.DeleteRuleFunc(accountID, ruleID)
	}
	return status.Errorf(codes.Unimplemented, "method DeleteRule not implemented")
}

func (am *MockAccountManager) ListRules(accountID string) ([]*server.Rule, error) {
	if am.ListRulesFunc != nil {
		return am.ListRulesFunc(accountID)
	}
	return nil, status.Errorf(codes.Unimplemented, "method ListRules not implemented")
}

func (am *MockAccountManager) UpdatePeerMeta(peerKey string, meta server.PeerSystemMeta) error {
	if am.UpdatePeerMetaFunc != nil {
		return am.UpdatePeerMetaFunc(peerKey, meta)
	}
	return status.Errorf(codes.Unimplemented, "method UpdatePeerMetaFunc not implemented")
}

func (am *MockAccountManager) IsUserAdmin(claims jwtclaims.AuthorizationClaims) (bool, error) {
	if am.IsUserAdminFunc != nil {
		return am.IsUserAdminFunc(claims)
	}
	return false, status.Errorf(codes.Unimplemented, "method IsUserAdmin not implemented")
}
