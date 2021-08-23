package server

import (
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync"
	"time"
)

type AccountManager struct {
	Store Store
	// mutex to synchronise account operations (e.g. generating Peer IP address inside the Network)
	mux sync.Mutex
}

// Account represents a unique account of the system
type Account struct {
	Id        string
	SetupKeys map[string]*SetupKey
	Network   *Network
	Peers     map[string]*Peer
}

// NewManager creates a new AccountManager with a provided Store
func NewManager(store Store) *AccountManager {
	return &AccountManager{
		Store: store,
		mux:   sync.Mutex{},
	}
}

//AddSetupKey generates a new setup key with a given name and type, and adds it to the specified account
func (manager *AccountManager) AddSetupKey(accountId string, keyName string, keyType SetupKeyType, expiresIn time.Duration) (*SetupKey, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	account, err := manager.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	setupKey := GenerateSetupKey(keyName, keyType, expiresIn)
	account.SetupKeys[setupKey.Key] = setupKey

	err = manager.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return setupKey, nil
}

//RevokeSetupKey marks SetupKey as revoked - becomes not valid anymore
func (manager *AccountManager) RevokeSetupKey(accountId string, keyId string) (*SetupKey, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	account, err := manager.Store.GetAccount(accountId)
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
	err = manager.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return keyCopy, nil
}

//RenameSetupKey renames existing setup key of the specified account.
func (manager *AccountManager) RenameSetupKey(accountId string, keyId string, newName string) (*SetupKey, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	account, err := manager.Store.GetAccount(accountId)
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
	err = manager.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed adding account key")
	}

	return keyCopy, nil
}

//GetAccount returns an existing account or error (NotFound) if doesn't exist
func (manager *AccountManager) GetAccount(accountId string) (*Account, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	account, err := manager.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found")
	}

	return account, nil
}

// GetOrCreateAccount returns an existing account or creates a new one if doesn't exist
func (manager *AccountManager) GetOrCreateAccount(accountId string) (*Account, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	_, err := manager.Store.GetAccount(accountId)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			return manager.createAccount(accountId)
		} else {
			// other error
			return nil, err
		}
	}

	account, err := manager.Store.GetAccount(accountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed retrieving account")
	}

	return account, nil
}

//AccountExists checks whether account exists (returns true) or not (returns false)
func (manager *AccountManager) AccountExists(accountId string) (*bool, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	var res bool
	_, err := manager.Store.GetAccount(accountId)
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
func (manager *AccountManager) AddAccount(accountId string) (*Account, error) {

	manager.mux.Lock()
	defer manager.mux.Unlock()

	return manager.createAccount(accountId)

}

func (manager *AccountManager) createAccount(accountId string) (*Account, error) {
	account, _ := newAccountWithId(accountId)

	err := manager.Store.SaveAccount(account)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed creating account")
	}

	return account, nil
}

// newAccountWithId creates a new Account with a default SetupKey (doesn't store in a Store) and provided id
func newAccountWithId(accountId string) (*Account, *SetupKey) {

	log.Debugf("creating new account")

	setupKeys := make(map[string]*SetupKey)
	setupKey := GenerateDefaultSetupKey()
	setupKeys[setupKey.Key] = setupKey
	network := &Network{
		Id:  uuid.New().String(),
		Net: net.IPNet{IP: net.ParseIP("100.64.0.0"), Mask: net.IPMask{255, 192, 0, 0}},
		Dns: ""}
	peers := make(map[string]*Peer)

	log.Debugf("created new account %s with setup key %s", accountId, setupKey.Key)

	return &Account{Id: accountId, SetupKeys: setupKeys, Network: network, Peers: peers}, setupKey
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
