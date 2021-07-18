package management

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wiretrustee/wiretrustee/util"
)

// storeFileName Store file name. Stored in the datadir
const storeFileName = "store.json"

// Store represents an account storage
type FileStore struct {
	Accounts             map[string]*Account
	SetupKeyId2AccountId map[string]string `json:"-"`

	// mutex to synchronise Store read/write operations
	mux       sync.Mutex `json:"-"`
	storeFile string     `json:"-"`
}

// NewStore restores a store from the file located in the datadir
func NewStore(dataDir string) (*FileStore, error) {
	return restore(filepath.Join(dataDir, storeFileName))
}

// restore restores the state of the store from the file.
// Creates a new empty store file if doesn't exist
func restore(file string) (*FileStore, error) {

	if _, err := os.Stat(file); os.IsNotExist(err) {
		// create a new FileStore if previously didn't exist (e.g. first run)
		s := &FileStore{
			Accounts:  make(map[string]*Account),
			mux:       sync.Mutex{},
			storeFile: file,
		}

		err = s.persist(file)
		if err != nil {
			return nil, err
		}

		return s, nil
	}

	read, err := util.ReadJson(file, &FileStore{})
	if err != nil {
		return nil, err
	}

	store := read.(*FileStore)
	store.storeFile = file
	store.SetupKeyId2AccountId = make(map[string]string)
	for accountId, account := range store.Accounts {
		for setupKeyId := range account.SetupKeys {
			store.SetupKeyId2AccountId[strings.ToLower(setupKeyId)] = accountId
		}
	}

	return store, nil
}

// persist persists account data to a file
// It is recommended to call it with locking FileStore.mux
func (s *FileStore) persist(file string) error {
	return util.WriteJson(file, s)
}

// AddPeer adds peer to the store and associates it with a Account and a SetupKey. Returns related Account
// Each Account has a list of pre-authorised SetupKey and if no Account has a given key nil will be returned, meaning the key is invalid
func (s *FileStore) AddPeer(setupKey string, peerKey string) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	accountId, accountIdFound := s.SetupKeyId2AccountId[strings.ToLower(setupKey)]
	if !accountIdFound {
		return status.Errorf(codes.NotFound, "Provided setup key doesn't exists")
	}

	account, accountFound := s.Accounts[accountId]
	if !accountFound {
		return status.Errorf(codes.Internal, "Invalid setup key")
	}

	key, setupKeyFound := account.SetupKeys[strings.ToLower(setupKey)]
	if !setupKeyFound {
		return status.Errorf(codes.Internal, "Invalid setup key")
	}

	account.Peers[peerKey] = &Peer{Key: peerKey, SetupKey: key}
	err := s.persist(s.storeFile)
	if err != nil {
		return err
	}
	return nil
}

// AddAccount adds new account to the store.
func (s *FileStore) AddAccount(account *Account) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	// todo will override, handle existing keys
	s.Accounts[account.Id] = account

	// todo check that account.Id and keyId are not exist already
	// because if keyId exists for other accounts this can be bad
	for keyId := range account.SetupKeys {
		s.SetupKeyId2AccountId[strings.ToLower(keyId)] = account.Id
	}

	err := s.persist(s.storeFile)
	if err != nil {
		return err
	}

	return nil
}
