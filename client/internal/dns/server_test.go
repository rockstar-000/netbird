package dns

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/netbirdio/netbird/client/internal/stdnet"

	"github.com/miekg/dns"

	nbdns "github.com/netbirdio/netbird/dns"
	"github.com/netbirdio/netbird/iface"
)

var zoneRecords = []nbdns.SimpleRecord{
	{
		Name:  "peera.netbird.cloud",
		Type:  1,
		Class: nbdns.DefaultClass,
		TTL:   300,
		RData: "1.2.3.4",
	},
}

func TestUpdateDNSServer(t *testing.T) {
	nameServers := []nbdns.NameServer{
		{
			IP:     netip.MustParseAddr("8.8.8.8"),
			NSType: nbdns.UDPNameServerType,
			Port:   53,
		},
		{
			IP:     netip.MustParseAddr("8.8.4.4"),
			NSType: nbdns.UDPNameServerType,
			Port:   53,
		},
	}

	dummyHandler := &localResolver{}

	testCases := []struct {
		name                string
		initUpstreamMap     registeredHandlerMap
		initLocalMap        registrationMap
		initSerial          uint64
		inputSerial         uint64
		inputUpdate         nbdns.Config
		shouldFail          bool
		expectedUpstreamMap registeredHandlerMap
		expectedLocalMap    registrationMap
	}{
		{
			name:            "Initial Config Should Succeed",
			initLocalMap:    make(registrationMap),
			initUpstreamMap: make(registeredHandlerMap),
			initSerial:      0,
			inputSerial:     1,
			inputUpdate: nbdns.Config{
				ServiceEnable: true,
				CustomZones: []nbdns.CustomZone{
					{
						Domain:  "netbird.cloud",
						Records: zoneRecords,
					},
				},
				NameServerGroups: []*nbdns.NameServerGroup{
					{
						Domains:     []string{"netbird.io"},
						NameServers: nameServers,
					},
					{
						NameServers: nameServers,
						Primary:     true,
					},
				},
			},
			expectedUpstreamMap: registeredHandlerMap{"netbird.io": dummyHandler, "netbird.cloud": dummyHandler, nbdns.RootZone: dummyHandler},
			expectedLocalMap:    registrationMap{buildRecordKey(zoneRecords[0].Name, 1, 1): struct{}{}},
		},
		{
			name:            "New Config Should Succeed",
			initLocalMap:    registrationMap{"netbird.cloud": struct{}{}},
			initUpstreamMap: registeredHandlerMap{buildRecordKey(zoneRecords[0].Name, 1, 1): dummyHandler},
			initSerial:      0,
			inputSerial:     1,
			inputUpdate: nbdns.Config{
				ServiceEnable: true,
				CustomZones: []nbdns.CustomZone{
					{
						Domain:  "netbird.cloud",
						Records: zoneRecords,
					},
				},
				NameServerGroups: []*nbdns.NameServerGroup{
					{
						Domains:     []string{"netbird.io"},
						NameServers: nameServers,
					},
				},
			},
			expectedUpstreamMap: registeredHandlerMap{"netbird.io": dummyHandler, "netbird.cloud": dummyHandler},
			expectedLocalMap:    registrationMap{buildRecordKey(zoneRecords[0].Name, 1, 1): struct{}{}},
		},
		{
			name:            "Smaller Config Serial Should Be Skipped",
			initLocalMap:    make(registrationMap),
			initUpstreamMap: make(registeredHandlerMap),
			initSerial:      2,
			inputSerial:     1,
			shouldFail:      true,
		},
		{
			name:            "Empty NS Group Domain Or Not Primary Element Should Fail",
			initLocalMap:    make(registrationMap),
			initUpstreamMap: make(registeredHandlerMap),
			initSerial:      0,
			inputSerial:     1,
			inputUpdate: nbdns.Config{
				ServiceEnable: true,
				CustomZones: []nbdns.CustomZone{
					{
						Domain:  "netbird.cloud",
						Records: zoneRecords,
					},
				},
				NameServerGroups: []*nbdns.NameServerGroup{
					{
						NameServers: nameServers,
					},
				},
			},
			shouldFail: true,
		},
		{
			name:            "Invalid NS Group Nameservers list Should Fail",
			initLocalMap:    make(registrationMap),
			initUpstreamMap: make(registeredHandlerMap),
			initSerial:      0,
			inputSerial:     1,
			inputUpdate: nbdns.Config{
				ServiceEnable: true,
				CustomZones: []nbdns.CustomZone{
					{
						Domain:  "netbird.cloud",
						Records: zoneRecords,
					},
				},
				NameServerGroups: []*nbdns.NameServerGroup{
					{
						NameServers: nameServers,
					},
				},
			},
			shouldFail: true,
		},
		{
			name:            "Invalid Custom Zone Records list Should Fail",
			initLocalMap:    make(registrationMap),
			initUpstreamMap: make(registeredHandlerMap),
			initSerial:      0,
			inputSerial:     1,
			inputUpdate: nbdns.Config{
				ServiceEnable: true,
				CustomZones: []nbdns.CustomZone{
					{
						Domain: "netbird.cloud",
					},
				},
				NameServerGroups: []*nbdns.NameServerGroup{
					{
						NameServers: nameServers,
						Primary:     true,
					},
				},
			},
			shouldFail: true,
		},
		{
			name:                "Empty Config Should Succeed and Clean Maps",
			initLocalMap:        registrationMap{"netbird.cloud": struct{}{}},
			initUpstreamMap:     registeredHandlerMap{zoneRecords[0].Name: dummyHandler},
			initSerial:          0,
			inputSerial:         1,
			inputUpdate:         nbdns.Config{ServiceEnable: true},
			expectedUpstreamMap: make(registeredHandlerMap),
			expectedLocalMap:    make(registrationMap),
		},
		{
			name:                "Disabled Service Should clean map",
			initLocalMap:        registrationMap{"netbird.cloud": struct{}{}},
			initUpstreamMap:     registeredHandlerMap{zoneRecords[0].Name: dummyHandler},
			initSerial:          0,
			inputSerial:         1,
			inputUpdate:         nbdns.Config{ServiceEnable: false},
			expectedUpstreamMap: make(registeredHandlerMap),
			expectedLocalMap:    make(registrationMap),
		},
	}

	for n, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			newNet, err := stdnet.NewNet(nil)
			if err != nil {
				t.Fatal(err)
			}
			wgIface, err := iface.NewWGIFace(fmt.Sprintf("utun230%d", n), fmt.Sprintf("100.66.100.%d/32", n+1), iface.DefaultMTU, nil, nil, newNet)
			if err != nil {
				t.Fatal(err)
			}
			err = wgIface.Create()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				err = wgIface.Close()
				if err != nil {
					t.Log(err)
				}
			}()
			dnsServer, err := NewDefaultServer(context.Background(), wgIface, "")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				err = dnsServer.hostManager.restoreHostDNS()
				if err != nil {
					t.Log(err)
				}
			}()

			dnsServer.dnsMuxMap = testCase.initUpstreamMap
			dnsServer.localResolver.registeredMap = testCase.initLocalMap
			dnsServer.updateSerial = testCase.initSerial
			// pretend we are running
			dnsServer.listenerIsRunning = true

			err = dnsServer.UpdateDNSServer(testCase.inputSerial, testCase.inputUpdate)
			if err != nil {
				if testCase.shouldFail {
					return
				}
				t.Fatalf("update dns server should not fail, got error: %v", err)
			}

			if len(dnsServer.dnsMuxMap) != len(testCase.expectedUpstreamMap) {
				t.Fatalf("update upstream failed, map size is different than expected, want %d, got %d", len(testCase.expectedUpstreamMap), len(dnsServer.dnsMuxMap))
			}

			for key := range testCase.expectedUpstreamMap {
				_, found := dnsServer.dnsMuxMap[key]
				if !found {
					t.Fatalf("update upstream failed, key %s was not found in the dnsMuxMap: %#v", key, dnsServer.dnsMuxMap)
				}
			}

			if len(dnsServer.localResolver.registeredMap) != len(testCase.expectedLocalMap) {
				t.Fatalf("update local failed, registered map size is different than expected, want %d, got %d", len(testCase.expectedLocalMap), len(dnsServer.localResolver.registeredMap))
			}

			for key := range testCase.expectedLocalMap {
				_, found := dnsServer.localResolver.registeredMap[key]
				if !found {
					t.Fatalf("update local failed, key %s was not found in the localResolver.registeredMap: %#v", key, dnsServer.localResolver.registeredMap)
				}
			}
		})
	}
}

func TestDNSServerStartStop(t *testing.T) {
	testCases := []struct {
		name     string
		addrPort string
	}{
		{
			name: "Should Pass With Port Discovery",
		},
		{
			name:     "Should Pass With Custom Port",
			addrPort: "127.0.0.1:3535",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dnsServer := getDefaultServerWithNoHostManager(t, testCase.addrPort)

			dnsServer.hostManager = newNoopHostMocker()
			dnsServer.Start()
			time.Sleep(100 * time.Millisecond)
			if !dnsServer.listenerIsRunning {
				t.Fatal("dns server listener is not running")
			}
			defer dnsServer.Stop()
			err := dnsServer.localResolver.registerRecord(zoneRecords[0])
			if err != nil {
				t.Error(err)
			}

			dnsServer.dnsMux.Handle("netbird.cloud", dnsServer.localResolver)

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{
						Timeout: time.Second * 5,
					}
					addr := fmt.Sprintf("%s:%d", dnsServer.runtimeIP, dnsServer.runtimePort)
					conn, err := d.DialContext(ctx, network, addr)
					if err != nil {
						t.Log(err)
						// retry test before exit, for slower systems
						return d.DialContext(ctx, network, addr)
					}

					return conn, nil
				},
			}

			ips, err := resolver.LookupHost(context.Background(), zoneRecords[0].Name)
			if err != nil {
				t.Fatalf("failed to connect to the server, error: %v", err)
			}

			if ips[0] != zoneRecords[0].RData {
				t.Fatalf("got a different IP from the server: want %s, got %s", zoneRecords[0].RData, ips[0])
			}

			dnsServer.Stop()
			ctx, cancel := context.WithTimeout(context.TODO(), time.Second*1)
			defer cancel()
			_, err = resolver.LookupHost(ctx, zoneRecords[0].Name)
			if err == nil {
				t.Fatalf("we should encounter an error when querying a stopped server")
			}
		})
	}
}

func TestDNSServerUpstreamDeactivateCallback(t *testing.T) {
	hostManager := &mockHostConfigurator{}
	server := DefaultServer{
		dnsMux: dns.DefaultServeMux,
		localResolver: &localResolver{
			registeredMap: make(registrationMap),
		},
		hostManager: hostManager,
		currentConfig: hostDNSConfig{
			domains: []domainConfig{
				{false, "domain0", false},
				{false, "domain1", false},
				{false, "domain2", false},
			},
		},
	}

	var domainsUpdate string
	hostManager.applyDNSConfigFunc = func(config hostDNSConfig) error {
		domains := []string{}
		for _, item := range config.domains {
			if item.disabled {
				continue
			}
			domains = append(domains, item.domain)
		}
		domainsUpdate = strings.Join(domains, ",")
		return nil
	}

	deactivate, reactivate := server.upstreamCallbacks(&nbdns.NameServerGroup{
		Domains: []string{"domain1"},
		NameServers: []nbdns.NameServer{
			{IP: netip.MustParseAddr("8.8.0.0"), NSType: nbdns.UDPNameServerType, Port: 53},
		},
	}, nil)

	deactivate()
	expected := "domain0,domain2"
	domains := []string{}
	for _, item := range server.currentConfig.domains {
		if item.disabled {
			continue
		}
		domains = append(domains, item.domain)
	}
	got := strings.Join(domains, ",")
	if expected != got {
		t.Errorf("expected domains list: %q, got %q", expected, got)
	}

	reactivate()
	expected = "domain0,domain1,domain2"
	domains = []string{}
	for _, item := range server.currentConfig.domains {
		if item.disabled {
			continue
		}
		domains = append(domains, item.domain)
	}
	got = strings.Join(domains, ",")
	if expected != got {
		t.Errorf("expected domains list: %q, got %q", expected, domainsUpdate)
	}
}

func getDefaultServerWithNoHostManager(t *testing.T, addrPort string) *DefaultServer {
	mux := dns.NewServeMux()

	var parsedAddrPort *netip.AddrPort
	if addrPort != "" {
		parsed, err := netip.ParseAddrPort(addrPort)
		if err != nil {
			t.Fatal(err)
		}
		parsedAddrPort = &parsed
	}

	dnsServer := &dns.Server{
		Net:     "udp",
		Handler: mux,
		UDPSize: 65535,
	}

	ctx, cancel := context.WithCancel(context.TODO())

	return &DefaultServer{
		ctx:       ctx,
		ctxCancel: cancel,
		server:    dnsServer,
		dnsMux:    mux,
		dnsMuxMap: make(registeredHandlerMap),
		localResolver: &localResolver{
			registeredMap: make(registrationMap),
		},
		customAddress: parsedAddrPort,
	}
}
