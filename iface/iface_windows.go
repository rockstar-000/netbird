package iface

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/driver"
)

// Create Creates a new Wireguard interface, sets a given IP and brings it up.
func (w *WGIface) Create() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	WintunStaticRequestedGUID, _ := windows.GenerateGUID()
	adapter, err := driver.CreateAdapter(w.name, "WireGuard", &WintunStaticRequestedGUID)
	if err != nil {
		err = fmt.Errorf("error creating adapter: %w", err)
		return err
	}
	w.netInterface = adapter
	err = adapter.SetAdapterState(driver.AdapterStateUp)
	if err != nil {
		return err
	}
	state, _ := adapter.LUID().GUID()
	log.Debugln("device guid: ", state.String())
	return w.assignAddr()
}

// GetInterfaceGUIDString returns an interface GUID string
func (w *WGIface) GetInterfaceGUIDString() (string, error) {
	if w.netInterface == nil {
		return "", fmt.Errorf("interface has not been initialized yet")
	}
	windowsDevice := w.netInterface.(*driver.Adapter)
	luid := windowsDevice.LUID()
	guid, err := luid.GUID()
	if err != nil {
		return "", err
	}
	return guid.String(), nil
}

// Close closes the tunnel interface
func (w *WGIface) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.netInterface == nil {
		return nil
	}

	return w.netInterface.Close()
}

// assignAddr Adds IP address to the tunnel interface and network route based on the range provided
func (w *WGIface) assignAddr() error {
	luid := w.netInterface.(*driver.Adapter).LUID()

	log.Debugf("adding address %s to interface: %s", w.address.IP, w.name)
	err := luid.SetIPAddresses([]net.IPNet{{w.address.IP, w.address.Network.Mask}})
	if err != nil {
		return err
	}

	return nil
}
