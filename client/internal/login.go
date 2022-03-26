package internal

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/netbirdio/netbird/client/system"
	mgm "github.com/netbirdio/netbird/management/client"
	mgmProto "github.com/netbirdio/netbird/management/proto"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Login(ctx context.Context, config *Config, setupKey string) error {
	backOff := &backoff.ExponentialBackOff{
		InitialInterval:     time.Second,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         2 * time.Second,
		MaxElapsedTime:      time.Second * 10,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}

	// validate our peer's Wireguard PRIVATE key
	myPrivateKey, err := wgtypes.ParseKey(config.PrivateKey)
	if err != nil {
		log.Errorf("failed parsing Wireguard key %s: [%s]", config.PrivateKey, err.Error())
		return err
	}

	var mgmTlsEnabled bool
	if config.ManagementURL.Scheme == "https" {
		mgmTlsEnabled = true
	}

	loginOp := func() error {
		log.Debugf("connecting to Management Service %s", config.ManagementURL.String())
		mgmClient, err := mgm.NewClient(ctx, config.ManagementURL.Host, myPrivateKey, mgmTlsEnabled)
		if err != nil {
			log.Errorf("failed connecting to Management Service %s %v", config.ManagementURL.String(), err)
			return err
		}
		log.Debugf("connected to management Service %s", config.ManagementURL.String())

		serverKey, err := mgmClient.GetServerPublicKey()
		if err != nil {
			log.Errorf("failed while getting Management Service public key: %v", err)
			return err
		}

		_, err = loginPeer(*serverKey, mgmClient, setupKey)
		if err != nil {
			log.Errorf("failed logging-in peer on Management Service : %v", err)
			return err
		}

		err = mgmClient.Close()
		if err != nil {
			log.Errorf("failed closing Management Service client: %v", err)
			return err
		}

		return nil
	}

	err = backoff.RetryNotify(loginOp, backOff, func(err error, duration time.Duration) {
		log.Warnf("retrying Login to the Management service in %v due to error %v", duration, err)
	})
	if err != nil {
		log.Errorf("exiting login retry loop due to unrecoverable error: %v", err)
		return err
	}

	return nil
}

// loginPeer attempts to login to Management Service. If peer wasn't registered, tries the registration flow.
func loginPeer(serverPublicKey wgtypes.Key, client *mgm.GrpcClient, setupKey string) (*mgmProto.LoginResponse, error) {
	loginResp, err := client.Login(serverPublicKey)
	if err != nil {
		if s, ok := status.FromError(err); ok && s.Code() == codes.PermissionDenied {
			log.Debugf("peer registration required")
			return registerPeer(serverPublicKey, client, setupKey)
		} else {
			return nil, err
		}
	}

	log.Info("peer has successfully logged-in to Management Service")

	return loginResp, nil
}

// registerPeer checks whether setupKey was provided via cmd line and if not then it prompts user to enter a key.
// Otherwise tries to register with the provided setupKey via command line.
func registerPeer(serverPublicKey wgtypes.Key, client *mgm.GrpcClient, setupKey string) (*mgmProto.LoginResponse, error) {
	validSetupKey, err := uuid.Parse(setupKey)
	if err != nil {
		return nil, err
	}

	log.Debugf("sending peer registration request to Management Service")
	info := system.GetInfo()
	loginResp, err := client.Register(serverPublicKey, validSetupKey.String(), info)
	if err != nil {
		log.Errorf("failed registering peer %v", err)
		return nil, err
	}

	log.Infof("peer has been successfully registered on Management Service")

	return loginResp, nil
}
