package server

import (
	"context"
	"fmt"
	"github.com/netbirdio/netbird/management/server/telemetry"
	gPeer "google.golang.org/grpc/peer"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/http/middleware"
	"github.com/netbirdio/netbird/management/server/jwtclaims"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/netbirdio/netbird/encryption"
	"github.com/netbirdio/netbird/management/proto"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc/codes"
	gRPCPeer "google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// GRPCServer an instance of a Management gRPC API server
type GRPCServer struct {
	accountManager AccountManager
	wgKey          wgtypes.Key
	proto.UnimplementedManagementServiceServer
	peersUpdateManager     *PeersUpdateManager
	config                 *Config
	turnCredentialsManager TURNCredentialsManager
	jwtMiddleware          *middleware.JWTMiddleware
	appMetrics             telemetry.AppMetrics
}

// NewServer creates a new Management server
func NewServer(config *Config, accountManager AccountManager, peersUpdateManager *PeersUpdateManager,
	turnCredentialsManager TURNCredentialsManager, appMetrics telemetry.AppMetrics) (*GRPCServer, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	var jwtMiddleware *middleware.JWTMiddleware

	if config.HttpConfig != nil && config.HttpConfig.AuthIssuer != "" && config.HttpConfig.AuthAudience != "" && validateURL(config.HttpConfig.AuthKeysLocation) {
		jwtMiddleware, err = middleware.NewJwtMiddleware(
			config.HttpConfig.AuthIssuer,
			config.HttpConfig.AuthAudience,
			config.HttpConfig.AuthKeysLocation)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to create new jwt middleware, err: %v", err)
		}
	} else {
		log.Debug("unable to use http config to create new jwt middleware")
	}

	if appMetrics != nil {
		// update gauge based on number of connected peers which is equal to open gRPC streams
		err = appMetrics.GRPCMetrics().RegisterConnectedStreams(func() int64 {
			return int64(len(peersUpdateManager.peerChannels))
		})
		if err != nil {
			return nil, err
		}
	}

	return &GRPCServer{
		wgKey: key,
		// peerKey -> event channel
		peersUpdateManager:     peersUpdateManager,
		accountManager:         accountManager,
		config:                 config,
		turnCredentialsManager: turnCredentialsManager,
		jwtMiddleware:          jwtMiddleware,
		appMetrics:             appMetrics,
	}, nil
}

func (s *GRPCServer) GetServerKey(ctx context.Context, req *proto.Empty) (*proto.ServerKeyResponse, error) {
	// todo introduce something more meaningful with the key expiration/rotation
	if s.appMetrics != nil {
		s.appMetrics.GRPCMetrics().CountGetKeyRequest()
	}
	now := time.Now().Add(24 * time.Hour)
	secs := int64(now.Second())
	nanos := int32(now.Nanosecond())
	expiresAt := &timestamp.Timestamp{Seconds: secs, Nanos: nanos}

	return &proto.ServerKeyResponse{
		Key:       s.wgKey.PublicKey().String(),
		ExpiresAt: expiresAt,
	}, nil
}

// Sync validates the existence of a connecting peer, sends an initial state (all available for the connecting peers) and
// notifies the connected peer of any updates (e.g. new peers under the same account)
func (s *GRPCServer) Sync(req *proto.EncryptedMessage, srv proto.ManagementService_SyncServer) error {
	if s.appMetrics != nil {
		s.appMetrics.GRPCMetrics().CountSyncRequest()
	}
	p, ok := gRPCPeer.FromContext(srv.Context())
	if ok {
		log.Debugf("Sync request from peer [%s] [%s]", req.WgPubKey, p.Addr.String())
	}

	peerKey, err := wgtypes.ParseKey(req.GetWgPubKey())
	if err != nil {
		log.Warnf("error while parsing peer's Wireguard public key %s on Sync request.", peerKey.String())
		return status.Errorf(codes.InvalidArgument, "provided wgPubKey %s is invalid", peerKey.String())
	}

	peer, err := s.accountManager.GetPeer(peerKey.String())
	if err != nil {
		p, _ := gPeer.FromContext(srv.Context())
		msg := status.Errorf(codes.PermissionDenied, "provided peer with the key wgPubKey %s is not registered, remote addr is %s", peerKey.String(), p.Addr.String())
		log.Debug(msg)
		return msg
	}

	syncReq := &proto.SyncRequest{}
	err = encryption.DecryptMessage(peerKey, s.wgKey, req.Body, syncReq)
	if err != nil {
		p, _ := gPeer.FromContext(srv.Context())
		msg := status.Errorf(codes.InvalidArgument, "invalid request message from %s,remote addr is %s", peerKey.String(), p.Addr.String())
		log.Debug(msg)
		return msg
	}

	err = s.sendInitialSync(peerKey, peer, srv)
	if err != nil {
		log.Debugf("error while sending initial sync for %s: %v", peerKey.String(), err)
		return err
	}

	updates := s.peersUpdateManager.CreateChannel(peerKey.String())
	err = s.accountManager.MarkPeerConnected(peerKey.String(), true)
	if err != nil {
		log.Warnf("failed marking peer as connected %s %v", peerKey, err)
	}

	if s.config.TURNConfig.TimeBasedCredentials {
		s.turnCredentialsManager.SetupRefresh(peerKey.String())
	}
	// keep a connection to the peer and send updates when available
	for {
		select {
		// condition when there are some updates
		case update, open := <-updates:
			if !open {
				log.Debugf("updates channel for peer %s was closed", peerKey.String())
				return nil
			}
			log.Debugf("recevied an update for peer %s", peerKey.String())

			encryptedResp, err := encryption.EncryptMessage(peerKey, s.wgKey, update.Update)
			if err != nil {
				return status.Errorf(codes.Internal, "failed processing update message")
			}

			err = srv.SendMsg(&proto.EncryptedMessage{
				WgPubKey: s.wgKey.PublicKey().String(),
				Body:     encryptedResp,
			})
			if err != nil {
				return status.Errorf(codes.Internal, "failed sending update message")
			}
			log.Debugf("sent an update to peer %s", peerKey.String())
		// condition when client <-> server connection has been terminated
		case <-srv.Context().Done():
			// happens when connection drops, e.g. client disconnects
			log.Debugf("stream of peer %s has been closed", peerKey.String())
			s.peersUpdateManager.CloseChannel(peerKey.String())
			s.turnCredentialsManager.CancelRefresh(peerKey.String())
			err = s.accountManager.MarkPeerConnected(peerKey.String(), false)
			if err != nil {
				log.Warnf("failed marking peer as disconnected %s %v", peerKey, err)
			}
			// todo stop turn goroutine
			return srv.Context().Err()
		}
	}
}

func (s *GRPCServer) registerPeer(peerKey wgtypes.Key, req *proto.LoginRequest) (*Peer, error) {
	var (
		reqSetupKey string
		userID      string
	)

	if req.GetJwtToken() != "" {
		log.Debugln("using jwt token to register peer")

		if s.jwtMiddleware == nil {
			return nil, status.Error(codes.Internal, "no jwt middleware set")
		}

		token, err := s.jwtMiddleware.ValidateAndParse(req.GetJwtToken())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "invalid jwt token, err: %v", err)
		}
		claims := jwtclaims.ExtractClaimsWithToken(token, s.config.HttpConfig.AuthAudience)
		_, err = s.accountManager.GetAccountFromToken(claims)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to fetch account with claims, err: %v", err)
		}
		userID = claims.UserId
	} else {
		log.Debugln("using setup key to register peer")
		reqSetupKey = req.GetSetupKey()
		userID = ""
	}

	meta := req.GetMeta()
	if meta == nil {
		return nil, status.Errorf(codes.InvalidArgument, "peer meta data was not provided")
	}

	var sshKey []byte
	if req.GetPeerKeys() != nil {
		sshKey = req.GetPeerKeys().GetSshPubKey()
	}

	peer, err := s.accountManager.AddPeer(reqSetupKey, userID, &Peer{
		Key:    peerKey.String(),
		Name:   meta.GetHostname(),
		SSHKey: string(sshKey),
		Meta: PeerSystemMeta{
			Hostname:  meta.GetHostname(),
			GoOS:      meta.GetGoOS(),
			Kernel:    meta.GetKernel(),
			Core:      meta.GetCore(),
			Platform:  meta.GetPlatform(),
			OS:        meta.GetOS(),
			WtVersion: meta.GetWiretrusteeVersion(),
			UIVersion: meta.GetUiVersion(),
		},
	})
	if err != nil {
		if e, ok := FromError(err); ok {
			switch e.Type() {
			case PreconditionFailed:
				return nil, status.Errorf(codes.FailedPrecondition, e.message)
			case AccountNotFound:
			case SetupKeyNotFound:
			case UserNotFound:
				return nil, status.Errorf(codes.NotFound, e.message)
			default:
			}
		}
		return nil, status.Errorf(codes.Internal, "failed registering new peer")
	}

	// todo move to DefaultAccountManager the code below
	networkMap, err := s.accountManager.GetNetworkMap(peer.Key)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unable to fetch network map after registering peer, error: %v", err)
	}
	// notify other peers of our registration
	for _, remotePeer := range networkMap.Peers {
		remotePeerNetworkMap, err := s.accountManager.GetNetworkMap(remotePeer.Key)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "unable to fetch network map after registering peer, error: %v", err)
		}

		update := toSyncResponse(s.config, remotePeer, nil, remotePeerNetworkMap)
		err = s.peersUpdateManager.SendUpdate(remotePeer.Key, &UpdateMessage{Update: update})
		if err != nil {
			// todo rethink if we should keep this return
			return nil, status.Errorf(codes.Internal, "unable to send update after registering peer, error: %v", err)
		}
	}

	return peer, nil
}

// Login endpoint first checks whether peer is registered under any account
// In case it is, the login is successful
// In case it isn't, the endpoint checks whether setup key is provided within the request and tries to register a peer.
// In case of the successful registration login is also successful
func (s *GRPCServer) Login(ctx context.Context, req *proto.EncryptedMessage) (*proto.EncryptedMessage, error) {
	if s.appMetrics != nil {
		s.appMetrics.GRPCMetrics().CountLoginRequest()
	}
	p, ok := gRPCPeer.FromContext(ctx)
	if ok {
		log.Debugf("Login request from peer [%s] [%s]", req.WgPubKey, p.Addr.String())
	}

	peerKey, err := wgtypes.ParseKey(req.GetWgPubKey())
	if err != nil {
		log.Warnf("error while parsing peer's Wireguard public key %s on Sync request.", req.WgPubKey)
		return nil, status.Errorf(codes.InvalidArgument, "provided wgPubKey %s is invalid", req.WgPubKey)
	}

	loginReq := &proto.LoginRequest{}
	err = encryption.DecryptMessage(peerKey, s.wgKey, req.Body, loginReq)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request message")
	}

	peer, err := s.accountManager.GetPeer(peerKey.String())
	if err != nil {
		if errStatus, ok := status.FromError(err); ok && errStatus.Code() == codes.NotFound {
			// peer doesn't exist -> check if setup key was provided
			if loginReq.GetJwtToken() == "" && loginReq.GetSetupKey() == "" {
				// absent setup key or jwt -> permission denied
				p, _ := gPeer.FromContext(ctx)
				msg := status.Errorf(codes.PermissionDenied,
					"provided peer with the key wgPubKey %s is not registered and no setup key or jwt was provided,"+
						" remote addr is %s", peerKey.String(), p.Addr.String())
				log.Debug(msg)
				return nil, msg
			}

			// setup key or jwt is present -> try normal registration flow
			peer, err = s.registerPeer(peerKey, loginReq)
			if err != nil {
				return nil, err
			}

		} else {
			return nil, status.Error(codes.Internal, "internal server error")
		}
	} else if loginReq.GetMeta() != nil {
		// update peer's system meta data on Login
		err = s.accountManager.UpdatePeerMeta(peerKey.String(), PeerSystemMeta{
			Hostname:  loginReq.GetMeta().GetHostname(),
			GoOS:      loginReq.GetMeta().GetGoOS(),
			Kernel:    loginReq.GetMeta().GetKernel(),
			Core:      loginReq.GetMeta().GetCore(),
			Platform:  loginReq.GetMeta().GetPlatform(),
			OS:        loginReq.GetMeta().GetOS(),
			WtVersion: loginReq.GetMeta().GetWiretrusteeVersion(),
			UIVersion: loginReq.GetMeta().GetUiVersion(),
		},
		)
		if err != nil {
			log.Errorf("failed updating peer system meta data %s", peerKey.String())
			return nil, status.Error(codes.Internal, "internal server error")
		}
	}

	var sshKey []byte
	if loginReq.GetPeerKeys() != nil {
		sshKey = loginReq.GetPeerKeys().GetSshPubKey()
	}

	if len(sshKey) > 0 {
		err = s.accountManager.UpdatePeerSSHKey(peerKey.String(), string(sshKey))
		if err != nil {
			return nil, err
		}
	}

	network, err := s.accountManager.GetPeerNetwork(peer.Key)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed getting peer network on login")
	}

	// if peer has reached this point then it has logged in
	loginResp := &proto.LoginResponse{
		WiretrusteeConfig: toWiretrusteeConfig(s.config, nil),
		PeerConfig:        toPeerConfig(peer, network),
	}
	encryptedResp, err := encryption.EncryptMessage(peerKey, s.wgKey, loginResp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed logging in peer")
	}

	return &proto.EncryptedMessage{
		WgPubKey: s.wgKey.PublicKey().String(),
		Body:     encryptedResp,
	}, nil
}

func ToResponseProto(configProto Protocol) proto.HostConfig_Protocol {
	switch configProto {
	case UDP:
		return proto.HostConfig_UDP
	case DTLS:
		return proto.HostConfig_DTLS
	case HTTP:
		return proto.HostConfig_HTTP
	case HTTPS:
		return proto.HostConfig_HTTPS
	case TCP:
		return proto.HostConfig_TCP
	default:
		// mbragin: todo something better?
		panic(fmt.Errorf("unexpected config protocol type %v", configProto))
	}
}

func toWiretrusteeConfig(config *Config, turnCredentials *TURNCredentials) *proto.WiretrusteeConfig {
	if config == nil {
		return nil
	}
	var stuns []*proto.HostConfig
	for _, stun := range config.Stuns {
		stuns = append(stuns, &proto.HostConfig{
			Uri:      stun.URI,
			Protocol: ToResponseProto(stun.Proto),
		})
	}
	var turns []*proto.ProtectedHostConfig
	for _, turn := range config.TURNConfig.Turns {
		var username string
		var password string
		if turnCredentials != nil {
			username = turnCredentials.Username
			password = turnCredentials.Password
		} else {
			username = turn.Username
			password = turn.Password
		}
		turns = append(turns, &proto.ProtectedHostConfig{
			HostConfig: &proto.HostConfig{
				Uri:      turn.URI,
				Protocol: ToResponseProto(turn.Proto),
			},
			User:     username,
			Password: password,
		})
	}

	return &proto.WiretrusteeConfig{
		Stuns: stuns,
		Turns: turns,
		Signal: &proto.HostConfig{
			Uri:      config.Signal.URI,
			Protocol: ToResponseProto(config.Signal.Proto),
		},
	}
}

func toPeerConfig(peer *Peer, network *Network) *proto.PeerConfig {
	netmask, _ := network.Net.Mask.Size()
	return &proto.PeerConfig{
		Address:   fmt.Sprintf("%s/%d", peer.IP.String(), netmask), // take it from the network
		SshConfig: &proto.SSHConfig{SshEnabled: peer.SSHEnabled},
	}
}

func toRemotePeerConfig(peers []*Peer) []*proto.RemotePeerConfig {
	remotePeers := []*proto.RemotePeerConfig{}
	for _, rPeer := range peers {
		remotePeers = append(remotePeers, &proto.RemotePeerConfig{
			WgPubKey:   rPeer.Key,
			AllowedIps: []string{fmt.Sprintf(AllowedIPsFormat, rPeer.IP)},
			SshConfig:  &proto.SSHConfig{SshPubKey: []byte(rPeer.SSHKey)},
		})
	}
	return remotePeers
}

func toSyncResponse(config *Config, peer *Peer, turnCredentials *TURNCredentials, networkMap *NetworkMap) *proto.SyncResponse {
	wtConfig := toWiretrusteeConfig(config, turnCredentials)

	pConfig := toPeerConfig(peer, networkMap.Network)

	remotePeers := toRemotePeerConfig(networkMap.Peers)

	routesUpdate := toProtocolRoutes(networkMap.Routes)

	dnsUpdate := toProtocolDNSConfig(networkMap.DNSConfig)

	return &proto.SyncResponse{
		WiretrusteeConfig:  wtConfig,
		PeerConfig:         pConfig,
		RemotePeers:        remotePeers,
		RemotePeersIsEmpty: len(remotePeers) == 0,
		NetworkMap: &proto.NetworkMap{
			Serial:             networkMap.Network.CurrentSerial(),
			PeerConfig:         pConfig,
			RemotePeers:        remotePeers,
			RemotePeersIsEmpty: len(remotePeers) == 0,
			Routes:             routesUpdate,
			DNSConfig:          dnsUpdate,
		},
	}
}

// IsHealthy indicates whether the service is healthy
func (s *GRPCServer) IsHealthy(ctx context.Context, req *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

// sendInitialSync sends initial proto.SyncResponse to the peer requesting synchronization
func (s *GRPCServer) sendInitialSync(peerKey wgtypes.Key, peer *Peer, srv proto.ManagementService_SyncServer) error {
	networkMap, err := s.accountManager.GetNetworkMap(peer.Key)
	if err != nil {
		log.Warnf("error getting a list of peers for a peer %s", peer.Key)
		return err
	}

	// make secret time based TURN credentials optional
	var turnCredentials *TURNCredentials
	if s.config.TURNConfig.TimeBasedCredentials {
		creds := s.turnCredentialsManager.GenerateCredentials()
		turnCredentials = &creds
	} else {
		turnCredentials = nil
	}
	plainResp := toSyncResponse(s.config, peer, turnCredentials, networkMap)

	encryptedResp, err := encryption.EncryptMessage(peerKey, s.wgKey, plainResp)
	if err != nil {
		return status.Errorf(codes.Internal, "error handling request")
	}

	err = srv.Send(&proto.EncryptedMessage{
		WgPubKey: s.wgKey.PublicKey().String(),
		Body:     encryptedResp,
	})

	if err != nil {
		log.Errorf("failed sending SyncResponse %v", err)
		return status.Errorf(codes.Internal, "error handling request")
	}

	return nil
}

// GetDeviceAuthorizationFlow returns a device authorization flow information
// This is used for initiating an Oauth 2 device authorization grant flow
// which will be used by our clients to Login
func (s *GRPCServer) GetDeviceAuthorizationFlow(ctx context.Context, req *proto.EncryptedMessage) (*proto.EncryptedMessage, error) {
	peerKey, err := wgtypes.ParseKey(req.GetWgPubKey())
	if err != nil {
		errMSG := fmt.Sprintf("error while parsing peer's Wireguard public key %s on GetDeviceAuthorizationFlow request.", req.WgPubKey)
		log.Warn(errMSG)
		return nil, status.Error(codes.InvalidArgument, errMSG)
	}

	err = encryption.DecryptMessage(peerKey, s.wgKey, req.Body, &proto.DeviceAuthorizationFlowRequest{})
	if err != nil {
		errMSG := fmt.Sprintf("error while decrypting peer's message with Wireguard public key %s.", req.WgPubKey)
		log.Warn(errMSG)
		return nil, status.Error(codes.InvalidArgument, errMSG)
	}

	if s.config.DeviceAuthorizationFlow == nil || s.config.DeviceAuthorizationFlow.Provider == string(NONE) {
		return nil, status.Error(codes.NotFound, "no device authorization flow information available")
	}

	provider, ok := proto.DeviceAuthorizationFlowProvider_value[strings.ToUpper(s.config.DeviceAuthorizationFlow.Provider)]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "no provider found in the protocol for %s", s.config.DeviceAuthorizationFlow.Provider)
	}

	flowInfoResp := &proto.DeviceAuthorizationFlow{
		Provider: proto.DeviceAuthorizationFlowProvider(provider),
		ProviderConfig: &proto.ProviderConfig{
			ClientID:           s.config.DeviceAuthorizationFlow.ProviderConfig.ClientID,
			ClientSecret:       s.config.DeviceAuthorizationFlow.ProviderConfig.ClientSecret,
			Domain:             s.config.DeviceAuthorizationFlow.ProviderConfig.Domain,
			Audience:           s.config.DeviceAuthorizationFlow.ProviderConfig.Audience,
			DeviceAuthEndpoint: s.config.DeviceAuthorizationFlow.ProviderConfig.DeviceAuthEndpoint,
			TokenEndpoint:      s.config.DeviceAuthorizationFlow.ProviderConfig.TokenEndpoint,
		},
	}

	encryptedResp, err := encryption.EncryptMessage(peerKey, s.wgKey, flowInfoResp)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to encrypt no device authorization flow information")
	}

	return &proto.EncryptedMessage{
		WgPubKey: s.wgKey.PublicKey().String(),
		Body:     encryptedResp,
	}, nil
}
