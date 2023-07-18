package acl

import (
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/firewall"
	"github.com/netbirdio/netbird/client/firewall/iptables"
	"github.com/netbirdio/netbird/client/firewall/nftables"
	"github.com/netbirdio/netbird/client/firewall/uspfilter"
)

// Create creates a firewall manager instance for the Linux
func Create(iface IFaceMapper) (manager *DefaultManager, err error) {
	var fm firewall.Manager
	if iface.IsUserspaceBind() {
		// use userspace packet filtering firewall
		if fm, err = uspfilter.Create(iface); err != nil {
			log.Debugf("failed to create userspace filtering firewall: %s", err)
			return nil, err
		}
	} else {
		if fm, err = nftables.Create(iface); err != nil {
			log.Debugf("failed to create nftables manager: %s", err)
			// fallback to iptables
			if fm, err = iptables.Create(iface); err != nil {
				log.Errorf("failed to create iptables manager: %s", err)
				return nil, err
			}
		}
	}

	return newDefaultManager(fm), nil
}
