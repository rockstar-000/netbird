package iface

import (
	"errors"
	"net"
	"time"

	log "github.com/sirupsen/logrus"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	errFuncNotImplemented = errors.New("function not implemented")
)

type wGConfigurer struct {
	tunDevice *tunDevice
}

func newWGConfigurer(tunDevice *tunDevice) wGConfigurer {
	return wGConfigurer{
		tunDevice: tunDevice,
	}
}

func (c *wGConfigurer) configureInterface(privateKey string, port int) error {
	log.Debugf("adding Wireguard private key")
	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return err
	}
	fwmark := 0
	config := wgtypes.Config{
		PrivateKey:   &key,
		ReplacePeers: true,
		FirewallMark: &fwmark,
		ListenPort:   &port,
	}

	return c.tunDevice.Device().IpcSet(toWgUserspaceString(config))
}

func (c *wGConfigurer) updatePeer(peerKey string, allowedIps string, keepAlive time.Duration, endpoint *net.UDPAddr, preSharedKey *wgtypes.Key) error {
	//parse allowed ips
	_, ipNet, err := net.ParseCIDR(allowedIps)
	if err != nil {
		return err
	}

	peerKeyParsed, err := wgtypes.ParseKey(peerKey)
	if err != nil {
		return err
	}
	peer := wgtypes.PeerConfig{
		PublicKey:                   peerKeyParsed,
		ReplaceAllowedIPs:           true,
		AllowedIPs:                  []net.IPNet{*ipNet},
		PersistentKeepaliveInterval: &keepAlive,
		PresharedKey:                preSharedKey,
		Endpoint:                    endpoint,
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	}

	return c.tunDevice.Device().IpcSet(toWgUserspaceString(config))
}

func (c *wGConfigurer) removePeer(peerKey string) error {
	peerKeyParsed, err := wgtypes.ParseKey(peerKey)
	if err != nil {
		return err
	}

	peer := wgtypes.PeerConfig{
		PublicKey: peerKeyParsed,
		Remove:    true,
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	}
	return c.tunDevice.Device().IpcSet(toWgUserspaceString(config))
}

func (c *wGConfigurer) addAllowedIP(peerKey string, allowedIP string) error {
	_, ipNet, err := net.ParseCIDR(allowedIP)
	if err != nil {
		return err
	}

	peerKeyParsed, err := wgtypes.ParseKey(peerKey)
	if err != nil {
		return err
	}
	peer := wgtypes.PeerConfig{
		PublicKey:         peerKeyParsed,
		UpdateOnly:        true,
		ReplaceAllowedIPs: false,
		AllowedIPs:        []net.IPNet{*ipNet},
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	}

	return c.tunDevice.Device().IpcSet(toWgUserspaceString(config))
}

func (c *wGConfigurer) removeAllowedIP(peerKey string, allowedIP string) error {
	return errFuncNotImplemented
}
