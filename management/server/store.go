package server

type Store interface {
	GetPeer(peerId string) (*Peer, error)
	GetAccount(accountId string) (*Account, error)
	GetPeerAccount(peerId string) (*Account, error)
	GetAccountBySetupKey(setupKey string) (*Account, error)
	SaveAccount(account *Account) error
}
