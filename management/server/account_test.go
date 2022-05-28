package server

import (
	"net"
	"testing"

	"github.com/netbirdio/netbird/management/server/jwtclaims"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestAccountManager_GetOrCreateAccountByUser(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	userId := "test_user"
	account, err := manager.GetOrCreateAccountByUser(userId, "")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		t.Fatalf("expected to create an account for a user %s", userId)
	}

	account, err = manager.GetAccountByUser(userId)
	if err != nil {
		t.Errorf("expected to get existing account after creation, no account was found for a user %s", userId)
	}

	if account != nil && account.Users[userId] == nil {
		t.Fatalf("expected to create an account for a user %s but no user was found after creation udner the account %s", userId, account.Id)
	}
}

func TestDefaultAccountManager_GetAccountWithAuthorizationClaims(t *testing.T) {
	type initUserParams jwtclaims.AuthorizationClaims

	type test struct {
		name                        string
		inputClaims                 jwtclaims.AuthorizationClaims
		inputInitUserParams         initUserParams
		inputUpdateAttrs            bool
		inputUpdateClaimAccount     bool
		testingFunc                 require.ComparisonAssertionFunc
		expectedMSG                 string
		expectedUserRole            UserRole
		expectedDomainCategory      string
		expectedPrimaryDomainStatus bool
	}

	var (
		publicDomain  = "public.com"
		privateDomain = "private.com"
		unknownDomain = "unknown.com"
	)

	defaultInitAccount := initUserParams{
		Domain: publicDomain,
		UserId: "defaultUser",
	}

	testCase1 := test{
		name: "New User With Public Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         publicDomain,
			UserId:         "pub-domain-user",
			DomainCategory: PublicCategory,
		},
		inputInitUserParams:         defaultInitAccount,
		testingFunc:                 require.NotEqual,
		expectedMSG:                 "account IDs shouldn't match",
		expectedUserRole:            UserRoleAdmin,
		expectedDomainCategory:      "",
		expectedPrimaryDomainStatus: false,
	}

	initUnknown := defaultInitAccount
	initUnknown.DomainCategory = UnknownCategory
	initUnknown.Domain = unknownDomain

	testCase2 := test{
		name: "New User With Unknown Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         unknownDomain,
			UserId:         "unknown-domain-user",
			DomainCategory: UnknownCategory,
		},
		inputInitUserParams:         initUnknown,
		testingFunc:                 require.NotEqual,
		expectedMSG:                 "account IDs shouldn't match",
		expectedUserRole:            UserRoleAdmin,
		expectedDomainCategory:      "",
		expectedPrimaryDomainStatus: false,
	}

	testCase3 := test{
		name: "New User With Private Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         privateDomain,
			UserId:         "pvt-domain-user",
			DomainCategory: PrivateCategory,
		},
		inputInitUserParams:         defaultInitAccount,
		testingFunc:                 require.NotEqual,
		expectedMSG:                 "account IDs shouldn't match",
		expectedUserRole:            UserRoleAdmin,
		expectedDomainCategory:      PrivateCategory,
		expectedPrimaryDomainStatus: true,
	}

	privateInitAccount := defaultInitAccount
	privateInitAccount.Domain = privateDomain
	privateInitAccount.DomainCategory = PrivateCategory

	testCase4 := test{
		name: "New Regular User With Existing Private Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         privateDomain,
			UserId:         "pvt-domain-user",
			DomainCategory: PrivateCategory,
		},
		inputUpdateAttrs:            true,
		inputInitUserParams:         privateInitAccount,
		testingFunc:                 require.Equal,
		expectedMSG:                 "account IDs should match",
		expectedUserRole:            UserRoleUser,
		expectedDomainCategory:      PrivateCategory,
		expectedPrimaryDomainStatus: true,
	}

	testCase5 := test{
		name: "Existing User With Existing Reclassified Private Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         defaultInitAccount.Domain,
			UserId:         defaultInitAccount.UserId,
			DomainCategory: PrivateCategory,
		},
		inputInitUserParams:         defaultInitAccount,
		testingFunc:                 require.Equal,
		expectedMSG:                 "account IDs should match",
		expectedUserRole:            UserRoleAdmin,
		expectedDomainCategory:      PrivateCategory,
		expectedPrimaryDomainStatus: true,
	}

	testCase6 := test{
		name: "Existing Account Id With Existing Reclassified Private Domain",
		inputClaims: jwtclaims.AuthorizationClaims{
			Domain:         defaultInitAccount.Domain,
			UserId:         defaultInitAccount.UserId,
			DomainCategory: PrivateCategory,
		},
		inputUpdateClaimAccount:     true,
		inputInitUserParams:         defaultInitAccount,
		testingFunc:                 require.Equal,
		expectedMSG:                 "account IDs should match",
		expectedUserRole:            UserRoleAdmin,
		expectedDomainCategory:      PrivateCategory,
		expectedPrimaryDomainStatus: true,
	}
	for _, testCase := range []test{testCase1, testCase2, testCase3, testCase4, testCase5, testCase6} {
		t.Run(testCase.name, func(t *testing.T) {
			manager, err := createManager(t)
			require.NoError(t, err, "unable to create account manager")

			initAccount, err := manager.GetAccountByUserOrAccountId(testCase.inputInitUserParams.UserId, testCase.inputInitUserParams.AccountId, testCase.inputInitUserParams.Domain)
			require.NoError(t, err, "create init user failed")

			if testCase.inputUpdateAttrs {
				err = manager.updateAccountDomainAttributes(initAccount, jwtclaims.AuthorizationClaims{UserId: testCase.inputInitUserParams.UserId, Domain: testCase.inputInitUserParams.Domain, DomainCategory: testCase.inputInitUserParams.DomainCategory}, true)
				require.NoError(t, err, "update init user failed")
			}

			if testCase.inputUpdateClaimAccount {
				testCase.inputClaims.AccountId = initAccount.Id
			}

			account, err := manager.GetAccountWithAuthorizationClaims(testCase.inputClaims)
			require.NoError(t, err, "support function failed")

			testCase.testingFunc(t, initAccount.Id, account.Id, testCase.expectedMSG)

			require.EqualValues(t, testCase.expectedUserRole, account.Users[testCase.inputClaims.UserId].Role, "expected user role should match")
			require.EqualValues(t, testCase.expectedDomainCategory, account.DomainCategory, "expected account domain category should match")
			require.EqualValues(t, testCase.expectedPrimaryDomainStatus, account.IsDomainPrimaryAccount, "expected account primary status should match")
		})
	}
}

func TestAccountManager_PrivateAccount(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	userId := "test_user"
	account, err := manager.GetOrCreateAccountByUser(userId, "")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		t.Fatalf("expected to create an account for a user %s", userId)
	}

	account, err = manager.GetAccountByUser(userId)
	if err != nil {
		t.Errorf("expected to get existing account after creation, no account was found for a user %s", userId)
	}

	if account != nil && account.Users[userId] == nil {
		t.Fatalf("expected to create an account for a user %s but no user was found after creation udner the account %s", userId, account.Id)
	}
}

func TestAccountManager_SetOrUpdateDomain(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	userId := "test_user"
	domain := "hotmail.com"
	account, err := manager.GetOrCreateAccountByUser(userId, domain)
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		t.Fatalf("expected to create an account for a user %s", userId)
	}

	if account.Domain != domain {
		t.Errorf("setting account domain failed, expected %s, got %s", domain, account.Domain)
	}

	domain = "gmail.com"

	account, err = manager.GetOrCreateAccountByUser(userId, domain)
	if err != nil {
		t.Fatalf("got the following error while retrieving existing acc: %v", err)
	}

	if account == nil {
		t.Fatalf("expected to get an account for a user %s", userId)
	}

	if account.Domain != domain {
		t.Errorf("updating domain. expected %s got %s", domain, account.Domain)
	}
}

func TestAccountManager_AddAccount(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	expectedId := "test_account"
	userId := "account_creator"
	expectedPeersSize := 0
	expectedSetupKeysSize := 2
	expectedNetwork := net.IPNet{
		IP:   net.IP{100, 64, 0, 0},
		Mask: net.IPMask{255, 255, 0, 0},
	}

	account, err := manager.AddAccount(expectedId, userId, "")
	if err != nil {
		t.Fatal(err)
	}

	if account.Id != expectedId {
		t.Errorf("expected account to have Id = %s, got %s", expectedId, account.Id)
	}

	if len(account.Peers) != expectedPeersSize {
		t.Errorf("expected account to have len(Peers) = %v, got %v", expectedPeersSize, len(account.Peers))
	}

	if len(account.SetupKeys) != expectedSetupKeysSize {
		t.Errorf("expected account to have len(SetupKeys) = %v, got %v", expectedSetupKeysSize, len(account.SetupKeys))
	}

	if account.Network.Net.String() != expectedNetwork.String() {
		t.Errorf("expected account to have Network = %v, got %v", expectedNetwork.String(), account.Network.Net.String())
	}
}

func TestAccountManager_GetAccountByUserOrAccountId(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	userId := "test_user"

	account, err := manager.GetAccountByUserOrAccountId(userId, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		t.Fatalf("expected to create an account for a user %s", userId)
	}

	accountId := account.Id

	_, err = manager.GetAccountByUserOrAccountId("", accountId, "")
	if err != nil {
		t.Errorf("expected to get existing account after creation using userid, no account was found for a account %s", accountId)
	}

	_, err = manager.GetAccountByUserOrAccountId("", "", "")
	if err == nil {
		t.Errorf("expected an error when user and account IDs are empty")
	}
}

func TestAccountManager_AccountExists(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	expectedId := "test_account"
	userId := "account_creator"
	_, err = manager.AddAccount(expectedId, userId, "")
	if err != nil {
		t.Fatal(err)
	}

	exists, err := manager.AccountExists(expectedId)
	if err != nil {
		t.Fatal(err)
	}

	if !*exists {
		t.Errorf("expected account to exist after creation, got false")
	}
}

func TestAccountManager_GetAccount(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	expectedId := "test_account"
	userId := "account_creator"
	account, err := manager.AddAccount(expectedId, userId, "")
	if err != nil {
		t.Fatal(err)
	}

	// AddAccount has been already tested so we can assume it is correct and compare results
	getAccount, err := manager.GetAccountById(expectedId)
	if err != nil {
		t.Fatal(err)
		return
	}

	if account.Id != getAccount.Id {
		t.Errorf("expected account.Id %s, got %s", account.Id, getAccount.Id)
	}

	for _, peer := range account.Peers {
		if _, ok := getAccount.Peers[peer.Key]; !ok {
			t.Errorf("expected account to have peer %s, not found", peer.Key)
		}
	}

	for _, key := range account.SetupKeys {
		if _, ok := getAccount.SetupKeys[key.Key]; !ok {
			t.Errorf("expected account to have setup key %s, not found", key.Key)
		}
	}
}

func TestAccountManager_AddPeer(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	account, err := manager.AddAccount("test_account", "account_creator", "")
	if err != nil {
		t.Fatal(err)
	}

	serial := account.Network.CurrentSerial() // should be 0

	var setupKey *SetupKey
	for _, key := range account.SetupKeys {
		setupKey = key
	}

	if setupKey == nil {
		t.Errorf("expecting account to have a default setup key")
		return
	}

	if account.Network.Serial != 0 {
		t.Errorf("expecting account network to have an initial Serial=0")
		return
	}

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
		return
	}
	expectedPeerKey := key.PublicKey().String()
	expectedPeerIP := "100.64.0.1"
	expectedSetupKey := setupKey.Key

	peer, err := manager.AddPeer(setupKey.Key, "", &Peer{
		Key:  expectedPeerKey,
		Meta: PeerSystemMeta{},
		Name: expectedPeerKey,
	})
	if err != nil {
		t.Errorf("expecting peer to be added, got failure %v", err)
		return
	}

	account, err = manager.GetAccountById(account.Id)
	if err != nil {
		t.Fatal(err)
		return
	}

	if peer.Key != expectedPeerKey {
		t.Errorf("expecting just added peer to have key = %s, got %s", expectedPeerKey, peer.Key)
	}

	if peer.IP.String() != expectedPeerIP {
		t.Errorf("expecting just added peer to have IP = %s, got %s", expectedPeerIP, peer.IP.String())
	}

	if peer.SetupKey != expectedSetupKey {
		t.Errorf("expecting just added peer to have SetupKey = %s, got %s", expectedSetupKey, peer.SetupKey)
	}

	if account.Network.CurrentSerial() != 1 {
		t.Errorf("expecting Network Serial=%d to be incremented by 1 and be equal to %d when adding new peer to account", serial, account.Network.CurrentSerial())
	}
}

func TestAccountManager_AddPeerWithUserID(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	userId := "account_creator"

	account, err := manager.GetOrCreateAccountByUser(userId, "")
	if err != nil {
		t.Fatal(err)
	}

	serial := account.Network.CurrentSerial() // should be 0

	if account.Network.Serial != 0 {
		t.Errorf("expecting account network to have an initial Serial=0")
		return
	}

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
		return
	}
	expectedPeerKey := key.PublicKey().String()
	expectedPeerIP := "100.64.0.1"
	expectedUserId := userId

	peer, err := manager.AddPeer("", userId, &Peer{
		Key:  expectedPeerKey,
		Meta: PeerSystemMeta{},
		Name: expectedPeerKey,
	})
	if err != nil {
		t.Errorf("expecting peer to be added, got failure %v, account users: %v", err, account.CreatedBy)
		return
	}

	account, err = manager.GetAccountById(account.Id)
	if err != nil {
		t.Fatal(err)
		return
	}

	if peer.Key != expectedPeerKey {
		t.Errorf("expecting just added peer to have key = %s, got %s", expectedPeerKey, peer.Key)
	}

	if peer.IP.String() != expectedPeerIP {
		t.Errorf("expecting just added peer to have IP = %s, got %s", expectedPeerIP, peer.IP.String())
	}

	if peer.UserID != expectedUserId {
		t.Errorf("expecting just added peer to have UserID = %s, got %s", expectedUserId, peer.UserID)
	}

	if account.Network.CurrentSerial() != 1 {
		t.Errorf("expecting Network Serial=%d to be incremented by 1 and be equal to %d when adding new peer to account", serial, account.Network.CurrentSerial())
	}
}

func TestAccountManager_DeletePeer(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	account, err := manager.AddAccount("test_account", "account_creator", "")
	if err != nil {
		t.Fatal(err)
	}

	var setupKey *SetupKey
	for _, key := range account.SetupKeys {
		setupKey = key
	}

	key, err := wgtypes.GenerateKey()
	if err != nil {
		t.Fatal(err)
		return
	}

	peerKey := key.PublicKey().String()

	_, err = manager.AddPeer(setupKey.Key, "", &Peer{
		Key:  peerKey,
		Meta: PeerSystemMeta{},
		Name: peerKey,
	})
	if err != nil {
		t.Errorf("expecting peer to be added, got failure %v", err)
		return
	}

	_, err = manager.DeletePeer(account.Id, peerKey)
	if err != nil {
		return
	}

	account, err = manager.GetAccountById(account.Id)
	if err != nil {
		t.Fatal(err)
		return
	}

	if account.Network.CurrentSerial() != 2 {
		t.Errorf("expecting Network Serial=%d to be incremented and be equal to 2 after adding and deleteing a peer", account.Network.CurrentSerial())
	}
}

func TestGetUsersFromAccount(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
	}

	users := map[string]*User{"1": {Id: "1", Role: "admin"}, "2": {Id: "2", Role: "user"}, "3": {Id: "3", Role: "user"}}
	accountId := "test_account_id"

	account, err := manager.AddAccount(accountId, users["1"].Id, "")
	if err != nil {
		t.Fatal(err)
	}

	// add a user to the account
	for _, user := range users {
		account.Users[user.Id] = user
	}

	userInfos, err := manager.GetUsersFromAccount(accountId)
	if err != nil {
		t.Fatal(err)
	}

	for _, userInfo := range userInfos {
		id := userInfo.ID
		assert.Equal(t, userInfo.ID, users[id].Id)
		assert.Equal(t, string(userInfo.Role), string(users[id].Role))
		assert.Equal(t, userInfo.Name, "")
		assert.Equal(t, userInfo.Email, "")
	}
}

func TestAccountManager_UpdatePeerMeta(t *testing.T) {
	manager, err := createManager(t)
	if err != nil {
		t.Fatal(err)
		return
	}

	account, err := manager.AddAccount("test_account", "account_creator", "")
	if err != nil {
		t.Fatal(err)
	}

	var setupKey *SetupKey
	for _, key := range account.SetupKeys {
		setupKey = key
	}

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
		return
	}

	peer, err := manager.AddPeer(setupKey.Key, "", &Peer{
		Key: key.PublicKey().String(),
		Meta: PeerSystemMeta{
			Hostname:  "Hostname",
			GoOS:      "GoOS",
			Kernel:    "Kernel",
			Core:      "Core",
			Platform:  "Platform",
			OS:        "OS",
			WtVersion: "WtVersion",
		},
		Name: key.PublicKey().String(),
	})
	if err != nil {
		t.Errorf("expecting peer to be added, got failure %v", err)
		return
	}

	newMeta := PeerSystemMeta{
		Hostname:  "new-Hostname",
		GoOS:      "new-GoOS",
		Kernel:    "new-Kernel",
		Core:      "new-Core",
		Platform:  "new-Platform",
		OS:        "new-OS",
		WtVersion: "new-WtVersion",
	}
	err = manager.UpdatePeerMeta(peer.Key, newMeta)
	if err != nil {
		t.Error(err)
		return
	}

	p, err := manager.GetPeer(peer.Key)
	if err != nil {
		return
	}

	if err != nil {
		t.Fatal(err)
		return
	}

	assert.Equal(t, newMeta, p.Meta)

}

func createManager(t *testing.T) (*DefaultAccountManager, error) {
	store, err := createStore(t)
	if err != nil {
		return nil, err
	}
	return BuildManager(store, NewPeersUpdateManager(), nil)
}

func createStore(t *testing.T) (Store, error) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		return nil, err
	}

	return store, nil
}
