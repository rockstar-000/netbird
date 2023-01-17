package server

import (
	"github.com/netbirdio/netbird/management/server/activity"
	"github.com/netbirdio/netbird/management/server/status"
	"github.com/stretchr/testify/require"
	"testing"
)

const (
	dnsGroup1ID      = "group1"
	dnsGroup2ID      = "group2"
	dnsPeer1Key      = "BhRPtynAAYRDy08+q4HTMsos8fs4plTP4NOSh7C1ry8="
	dnsPeer2Key      = "/yF0+vCfv+mRR5k0dca0TrGdO/oiNeAI58gToZm5NyI="
	dnsAccountID     = "testingAcc"
	dnsAdminUserID   = "testingAdminUser"
	dnsRegularUserID = "testingRegularUser"
)

func TestGetDNSSettings(t *testing.T) {
	am, err := createDNSManager(t)
	if err != nil {
		t.Error("failed to create account manager")
	}

	account, err := initTestDNSAccount(t, am)
	if err != nil {
		t.Error("failed to init testing account")
	}

	dnsSettings, err := am.GetDNSSettings(account.Id, dnsAdminUserID)
	if err != nil {
		t.Fatalf("Got an error when trying to retrieve the DNS settings with an admin user, err: %s", err)
	}

	if dnsSettings == nil {
		t.Fatal("DNS settings for new accounts shouldn't return nil")
	}

	account.DNSSettings = &DNSSettings{
		DisabledManagementGroups: []string{group1ID},
	}

	err = am.Store.SaveAccount(account)
	if err != nil {
		t.Error("failed to save testing account with new DNS settings")
	}

	dnsSettings, err = am.GetDNSSettings(account.Id, dnsAdminUserID)
	if err != nil {
		t.Errorf("Got an error when trying to retrieve the DNS settings with an admin user, err: %s", err)
	}

	if len(dnsSettings.DisabledManagementGroups) != 1 {
		t.Errorf("DNS settings should have one disabled mgmt group, groups: %s", dnsSettings.DisabledManagementGroups)
	}

	_, err = am.GetDNSSettings(account.Id, dnsRegularUserID)
	if err == nil {
		t.Errorf("An error should be returned when getting the DNS settings with a regular user")
	}

	s, ok := status.FromError(err)
	if !ok && s.Type() != status.PermissionDenied {
		t.Errorf("returned error should be Permission Denied, got err: %s", err)
	}
}

func TestSaveDNSSettings(t *testing.T) {
	testCases := []struct {
		name          string
		userID        string
		inputSettings *DNSSettings
		shouldFail    bool
	}{
		{
			name:   "Saving As Admin Should Be OK",
			userID: dnsAdminUserID,
			inputSettings: &DNSSettings{
				DisabledManagementGroups: []string{dnsGroup1ID},
			},
		},
		{
			name:   "Should Not Update Settings As Regular User",
			userID: dnsRegularUserID,
			inputSettings: &DNSSettings{
				DisabledManagementGroups: []string{dnsGroup1ID},
			},
			shouldFail: true,
		},
		{
			name:          "Should Not Update Settings If Input is Nil",
			userID:        dnsAdminUserID,
			inputSettings: nil,
			shouldFail:    true,
		},
		{
			name:   "Should Not Update Settings If Group Is Invalid",
			userID: dnsAdminUserID,
			inputSettings: &DNSSettings{
				DisabledManagementGroups: []string{"non-existing-group"},
			},
			shouldFail: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			am, err := createDNSManager(t)
			if err != nil {
				t.Error("failed to create account manager")
			}

			account, err := initTestDNSAccount(t, am)
			if err != nil {
				t.Error("failed to init testing account")
			}

			err = am.SaveDNSSettings(account.Id, testCase.userID, testCase.inputSettings)
			if err != nil {
				if testCase.shouldFail {
					return
				}
				t.Error(err)
			}

			updatedAccount, err := am.Store.GetAccount(account.Id)
			if err != nil {
				t.Errorf("should be able to retrieve updated account, got err: %s", err)
			}

			require.ElementsMatchf(t, testCase.inputSettings.DisabledManagementGroups, updatedAccount.DNSSettings.DisabledManagementGroups,
				"resulting DNS settings should match input")

		})
	}
}

func TestGetNetworkMap_DNSConfigSync(t *testing.T) {

	am, err := createDNSManager(t)
	if err != nil {
		t.Error("failed to create account manager")
	}

	account, err := initTestDNSAccount(t, am)
	if err != nil {
		t.Error("failed to init testing account")
	}

	newAccountDNSConfig, err := am.GetNetworkMap(dnsPeer1Key)
	require.NoError(t, err)
	require.Len(t, newAccountDNSConfig.DNSConfig.CustomZones, 1, "default DNS config should have one custom zone for peers")
	require.True(t, newAccountDNSConfig.DNSConfig.ServiceEnable, "default DNS config should have local DNS service enabled")

	dnsSettings := account.DNSSettings.Copy()
	dnsSettings.DisabledManagementGroups = append(dnsSettings.DisabledManagementGroups, dnsGroup1ID)
	account.DNSSettings = dnsSettings
	err = am.Store.SaveAccount(account)
	require.NoError(t, err)

	updatedAccountDNSConfig, err := am.GetNetworkMap(dnsPeer1Key)
	require.NoError(t, err)
	require.Len(t, updatedAccountDNSConfig.DNSConfig.CustomZones, 0, "updated DNS config should have no custom zone when peer belongs to a disabled group")
	require.False(t, updatedAccountDNSConfig.DNSConfig.ServiceEnable, "updated DNS config should have local DNS service disabled when peer belongs to a disabled group")

	peer2AccountDNSConfig, err := am.GetNetworkMap(dnsPeer2Key)
	require.NoError(t, err)
	require.Len(t, peer2AccountDNSConfig.DNSConfig.CustomZones, 1, "DNS config should have one custom zone for peers not in the disabled group")
	require.True(t, peer2AccountDNSConfig.DNSConfig.ServiceEnable, "DNS config should have DNS service enabled for peers not in the disabled group")
}

func createDNSManager(t *testing.T) (*DefaultAccountManager, error) {
	store, err := createDNSStore(t)
	if err != nil {
		return nil, err
	}
	eventStore := &activity.InMemoryEventStore{}
	return BuildManager(store, NewPeersUpdateManager(), nil, "", "netbird.test", eventStore)
}

func createDNSStore(t *testing.T) (Store, error) {
	dataDir := t.TempDir()
	store, err := NewFileStore(dataDir)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func initTestDNSAccount(t *testing.T, am *DefaultAccountManager) (*Account, error) {
	peer1 := &Peer{
		Key:  dnsPeer1Key,
		Name: "test-host1@netbird.io",
		Meta: PeerSystemMeta{
			Hostname:  "test-host1@netbird.io",
			GoOS:      "linux",
			Kernel:    "Linux",
			Core:      "21.04",
			Platform:  "x86_64",
			OS:        "Ubuntu",
			WtVersion: "development",
			UIVersion: "development",
		},
		DNSLabel: dnsPeer1Key,
	}
	peer2 := &Peer{
		Key:  dnsPeer2Key,
		Name: "test-host2@netbird.io",
		Meta: PeerSystemMeta{
			Hostname:  "test-host2@netbird.io",
			GoOS:      "linux",
			Kernel:    "Linux",
			Core:      "21.04",
			Platform:  "x86_64",
			OS:        "Ubuntu",
			WtVersion: "development",
			UIVersion: "development",
		},
		DNSLabel: dnsPeer2Key,
	}

	domain := "example.com"

	account := newAccountWithId(dnsAccountID, dnsAdminUserID, domain)

	account.Users[dnsRegularUserID] = &User{
		Id:   dnsRegularUserID,
		Role: UserRoleUser,
	}

	err := am.Store.SaveAccount(account)
	if err != nil {
		return nil, err
	}

	newGroup1 := &Group{
		ID:    dnsGroup1ID,
		Peers: []string{peer1.Key},
		Name:  dnsGroup1ID,
	}

	newGroup2 := &Group{
		ID:   dnsGroup2ID,
		Name: dnsGroup2ID,
	}

	account.Groups[newGroup1.ID] = newGroup1
	account.Groups[newGroup2.ID] = newGroup2

	err = am.Store.SaveAccount(account)
	if err != nil {
		return nil, err
	}

	_, err = am.AddPeer("", dnsAdminUserID, peer1)
	if err != nil {
		return nil, err
	}
	_, err = am.AddPeer("", dnsAdminUserID, peer2)
	if err != nil {
		return nil, err
	}

	return account, nil
}
