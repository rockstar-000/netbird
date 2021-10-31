package server

import (
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/wiretrustee/wiretrustee/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync"
)

type AccountManager struct {
	Store Store
	// mutex to synchronise account operations (e.g. generating Peer IP address inside the Network)
	mux                sync.Mutex
	peersUpdateManager *PeersUpdateManager
}

// Account represents a unique account of the system
type Account struct {
	Id        string
	SetupKeys map[string]*SetupKey
	Network   *Network
	Peers     map[string]*Peer
}

// NewManager creates a new AccountManager with a provided Store
func NewManager(store Store, peersUpdateManager *PeersUpdateManager) *AccountManager {
	return &AccountManager{
		Store:              store,
		mux:                sync.Mutex{},
		peersUpdateManager: peersUpdateManager,
	}
}

//AddSetupKey generates a new setup key with a given name and type, and adds it to the specified account
func (am *AccountManager) AddSetupKey(accountId string, keyName string, keyType SetupKeyType, expiresIn *util.Duration) (*SetupKey, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	keyDuration := DefaultSetupKeyDuration
	if expiresIn != nil {
		keyDuration = expiresIn.Duration
	}

	account, err := am.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	setupKey := GenerateSetupKey(keyName, keyType, keyDuration)
	account.SetupKeys[setupKey.Key] = setupKey

	err = am.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return setupKey, nil
}

//RevokeSetupKey marks SetupKey as revoked - becomes not valid anymore
func (am *AccountManager) RevokeSetupKey(accountId string, keyId string) (*SetupKey, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	setupKey := getAccountSetupKeyById(account, keyId)
	if setupKey == nil {
		return nil, status.Errorf(codes.NotFound, "unknown setupKey %s", keyId)
	}

	keyCopy := setupKey.Copy()
	keyCopy.Revoked = true
	account.SetupKeys[keyCopy.Key] = keyCopy
	err = am.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return keyCopy, nil
}

//RenameSetupKey renames existing setup key of the specified account.
func (am *AccountManager) RenameSetupKey(accountId string, keyId string, newName string) (*SetupKey, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	setupKey := getAccountSetupKeyById(account, keyId)
	if setupKey == nil {
		return nil, status.Errorf(codes.NotFound, "unknown setupKey %s", keyId)
	}

	keyCopy := setupKey.Copy()
	keyCopy.Name = newName
	account.SetupKeys[keyCopy.Key] = keyCopy
	err = am.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return keyCopy, nil
}

//GetAccount returns an existing account or error (NotFound) if doesn't exist
func (am *AccountManager) GetAccount(accountId string) (*Account, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	account, err := am.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	return account, nil
}

// GetOrCreateAccount returns an existing account or creates a new one if doesn't exist
func (am *AccountManager) GetOrCreateAccount(accountId string) (*Account, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	_, err := am.Store.GetAccount(accountId)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return am.createAccount(accountId)
		} else {
			// other error
			return nil, err
		}
	}

	account, err := am.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed retrieving account")
	}

	return account, nil
}

//AccountExists checks whether account exists (returns true) or not (returns false)
func (am *AccountManager) AccountExists(accountId string) (*bool, error) {
	am.mux.Lock()
	defer am.mux.Unlock()

	var res bool
	_, err := am.Store.GetAccount(accountId)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			res = false
			return &res, nil
		} else {
			return nil, err
		}
	}

	res = true
	return &res, nil
}

// AddAccount generates a new Account with a provided accountId and saves to the Store
func (am *AccountManager) AddAccount(accountId string) (*Account, error) {

	am.mux.Lock()
	defer am.mux.Unlock()

	return am.createAccount(accountId)

}

func (am *AccountManager) createAccount(accountId string) (*Account, error) {
	account, _ := newAccountWithId(accountId)

	err := am.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed creating account")
	}

	return account, nil
}

// newAccountWithId creates a new Account with a default SetupKey (doesn't store in a Store) and provided id
func newAccountWithId(accountId string) (*Account, *SetupKey) {

	log.Debugf("creating new account")

	setupKeys := make(map[string]*SetupKey)
	defaultKey := GenerateDefaultSetupKey()
	oneOffKey := GenerateSetupKey("One-off key", SetupKeyOneOff, DefaultSetupKeyDuration)
	setupKeys[defaultKey.Key] = defaultKey
	setupKeys[oneOffKey.Key] = oneOffKey
	network := &Network{
		Id:  uuid.New().String(),
		Net: net.IPNet{IP: net.ParseIP("100.64.0.0"), Mask: net.IPMask{255, 192, 0, 0}},
		Dns: ""}
	peers := make(map[string]*Peer)

	log.Debugf("created new account %s with setup key %s", accountId, defaultKey.Key)

	return &Account{Id: accountId, SetupKeys: setupKeys, Network: network, Peers: peers}, defaultKey
}

// newAccount creates a new Account with a default SetupKey (doesn't store in a Store)
func newAccount() (*Account, *SetupKey) {
	accountId := uuid.New().String()
	return newAccountWithId(accountId)
}

func getAccountSetupKeyById(acc *Account, keyId string) *SetupKey {
	for _, k := range acc.SetupKeys {
		if keyId == k.Id {
			return k
		}
	}
	return nil
}

func getAccountSetupKeyByKey(acc *Account, key string) *SetupKey {
	for _, k := range acc.SetupKeys {
		if key == k.Key {
			return k
		}
	}
	return nil
}
