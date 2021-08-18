package client

import (
	"context"
	"crypto/tls"
	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"github.com/wiretrustee/wiretrustee/encryption"
	"github.com/wiretrustee/wiretrustee/management/proto"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"io"
	"time"
)

type Client struct {
	key        wgtypes.Key
	realClient proto.ManagementServiceClient
	ctx        context.Context
	conn       *grpc.ClientConn
}

// NewClient creates a new client to Management service
func NewClient(ctx context.Context, addr string, ourPrivateKey wgtypes.Key, tlsEnabled bool) (*Client, error) {

	transportOption := grpc.WithInsecure()

	if tlsEnabled {
		transportOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{}))
	}

	mgmCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(
		mgmCtx,
		addr,
		transportOption,
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    3 * time.Second,
			Timeout: 2 * time.Second,
		}))

	if err != nil {
		log.Errorf("failed creating connection to Management Srvice %v", err)
		return nil, err
	}

	realClient := proto.NewManagementServiceClient(conn)

	return &Client{
		key:        ourPrivateKey,
		realClient: realClient,
		ctx:        ctx,
		conn:       conn,
	}, nil
}

// Close closes connection to the Management Service
func (c *Client) Close() error {
	return c.conn.Close()
}

// Sync wraps the real client's Sync endpoint call and takes care of retries and encryption/decryption of messages
// Non blocking request (executed in go routine). The result will be sent via msgHandler callback function
func (c *Client) Sync(msgHandler func(msg *proto.SyncResponse) error) {

	go func() {

		var backOff = &backoff.ExponentialBackOff{
			InitialInterval:     800 * time.Millisecond,
			RandomizationFactor: backoff.DefaultRandomizationFactor,
			Multiplier:          backoff.DefaultMultiplier,
			MaxInterval:         3 * time.Second,
			MaxElapsedTime:      time.Duration(0), //never stop retrying
			Stop:                backoff.Stop,
			Clock:               backoff.SystemClock,
		}

		operation := func() error {

			// todo we already have it since we did the Login, maybe cache it locally?
			serverPubKey, err := c.GetServerPublicKey()
			if err != nil {
				log.Errorf("failed getting Management Service public key: %s", err)
				return err
			}

			stream, err := c.connectToStream(*serverPubKey)
			if err != nil {
				log.Errorf("failed to open Management Service stream: %s", err)
				return err
			}

			log.Infof("connected to the Management Service Stream")

			// blocking until error
			err = c.receiveEvents(stream, *serverPubKey, msgHandler)
			if err != nil {
				return err
			}
			backOff.Reset()
			return nil
		}

		err := backoff.Retry(operation, backOff)
		if err != nil {
			log.Errorf("failed communicating with Management Service %s ", err)
			return
		}
	}()
}

func (c *Client) connectToStream(serverPubKey wgtypes.Key) (proto.ManagementService_SyncClient, error) {
	req := &proto.SyncRequest{}

	myPrivateKey := c.key
	myPublicKey := myPrivateKey.PublicKey()

	encryptedReq, err := encryption.EncryptMessage(serverPubKey, myPrivateKey, req)
	if err != nil {
		log.Errorf("failed encrypting message: %s", err)
		return nil, err
	}

	syncReq := &proto.EncryptedMessage{WgPubKey: myPublicKey.String(), Body: encryptedReq}
	return c.realClient.Sync(c.ctx, syncReq)
}

func (c *Client) receiveEvents(stream proto.ManagementService_SyncClient, serverPubKey wgtypes.Key, msgHandler func(msg *proto.SyncResponse) error) error {
	for {
		update, err := stream.Recv()
		if err == io.EOF {
			log.Errorf("managment stream was closed: %s", err)
			return err
		}
		if err != nil {
			log.Errorf("disconnected from Management Service syn stream: %v", err)
			return err
		}

		log.Debugf("got an update message from Management Service")
		decryptedResp := &proto.SyncResponse{}
		err = encryption.DecryptMessage(serverPubKey, c.key, update.Body, decryptedResp)
		if err != nil {
			log.Errorf("failed decrypting update message from Management Service: %s", err)
			return err
		}

		err = msgHandler(decryptedResp)
		if err != nil {
			log.Errorf("failed handling an update message received from Management Service %v", err.Error())
			return err
		}
	}
}

// GetServerPublicKey returns server Wireguard public key (used later for encrypting messages sent to the server)
func (c *Client) GetServerPublicKey() (*wgtypes.Key, error) {
	mgmCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second) //todo make a general setting
	defer cancel()
	resp, err := c.realClient.GetServerKey(mgmCtx, &proto.Empty{})
	if err != nil {
		return nil, err
	}

	serverKey, err := wgtypes.ParseKey(resp.Key)
	if err != nil {
		return nil, err
	}

	return &serverKey, nil
}

func (c *Client) login(serverKey wgtypes.Key, req *proto.LoginRequest) (*proto.LoginResponse, error) {
	loginReq, err := encryption.EncryptMessage(serverKey, c.key, req)
	if err != nil {
		log.Errorf("failed to encrypt message: %s", err)
		return nil, err
	}
	mgmCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second) //todo make a general setting
	defer cancel()
	resp, err := c.realClient.Login(mgmCtx, &proto.EncryptedMessage{
		WgPubKey: c.key.PublicKey().String(),
		Body:     loginReq,
	})

	if err != nil {
		return nil, err
	}

	loginResp := &proto.LoginResponse{}
	err = encryption.DecryptMessage(serverKey, c.key, resp.Body, loginResp)
	if err != nil {
		log.Errorf("failed to decrypt registration message: %s", err)
		return nil, err
	}

	return loginResp, nil
}

// Register registers peer on Management Server. It actually calls a Login endpoint with a provided setup key
// Takes care of encrypting and decrypting messages.
func (c *Client) Register(serverKey wgtypes.Key, setupKey string) (*proto.LoginResponse, error) {
	return c.login(serverKey, &proto.LoginRequest{SetupKey: setupKey})
}

// Login attempts login to Management Server. Takes care of encrypting and decrypting messages.
func (c *Client) Login(serverKey wgtypes.Key) (*proto.LoginResponse, error) {
	return c.login(serverKey, &proto.LoginRequest{})
}
