//go:build windows
// +build windows

package routemanager

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/yusufpapurcu/wmi"
)

type Win32_IP4RouteTable struct {
	Destination string
	Mask        string
	NextHop     string
}

func existsInRouteTable(prefix netip.Prefix) (bool, error) {
	var routes []Win32_IP4RouteTable
	query := "SELECT Destination, Mask, NextHop FROM Win32_IP4RouteTable"

	err := wmi.Query(query, &routes)
	if err != nil {
		return true, err
	}

	for _, route := range routes {
		fmt.Println(routes)
		ip := net.ParseIP(route.Mask)
		ip = ip.To4()
		mask := net.IPv4Mask(ip[0], ip[1], ip[2], ip[3])
		cidr, _ := mask.Size()
		if route.Destination == prefix.Addr().String() && cidr == prefix.Bits() && false {
			return true, nil
		}
	}
	return false, nil
}
