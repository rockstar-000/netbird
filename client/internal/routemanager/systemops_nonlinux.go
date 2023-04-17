//go:build !linux
// +build !linux

package routemanager

import (
	"net/netip"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"
)

func addToRouteTable(prefix netip.Prefix, addr string) error {
	cmd := exec.Command("route", "add", prefix.String(), addr)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	log.Debugf(string(out))
	return nil
}

func removeFromRouteTable(prefix netip.Prefix) error {
	cmd := exec.Command("route", "delete", prefix.String())
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	log.Debugf(string(out))
	return nil
}

func enableIPForwarding() error {
	log.Infof("enable IP forwarding is not implemented on %s", runtime.GOOS)
	return nil
}
