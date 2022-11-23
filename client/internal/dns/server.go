package dns

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	nbdns "github.com/netbirdio/netbird/dns"
	"github.com/netbirdio/netbird/iface"
	log "github.com/sirupsen/logrus"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"time"
)

const (
	port       = 53
	customPort = 5053
	defaultIP  = "127.0.0.1"
)

// Server is a dns server interface
type Server interface {
	Start()
	Stop()
	UpdateDNSServer(serial uint64, update nbdns.Config) error
}

// DefaultServer dns server object
type DefaultServer struct {
	ctx               context.Context
	stop              context.CancelFunc
	mux               sync.Mutex
	server            *dns.Server
	dnsMux            *dns.ServeMux
	dnsMuxMap         registrationMap
	localResolver     *localResolver
	wgInterface       *iface.WGIface
	hostManager       hostManager
	updateSerial      uint64
	listenerIsRunning bool
	runtimePort       int
	runtimeIP         string
}

type registrationMap map[string]struct{}

type muxUpdate struct {
	domain  string
	handler dns.Handler
}

// NewDefaultServer returns a new dns server
func NewDefaultServer(ctx context.Context, wgInterface *iface.WGIface) (*DefaultServer, error) {
	mux := dns.NewServeMux()
	listenIP := defaultIP
	if runtime.GOOS != "darwin" && wgInterface != nil {
		listenIP = wgInterface.GetAddress().IP.String()
	}

	dnsServer := &dns.Server{
		Addr:    fmt.Sprintf("%s:%d", listenIP, port),
		Net:     "udp",
		Handler: mux,
		UDPSize: 65535,
	}

	ctx, stop := context.WithCancel(ctx)

	defaultServer := &DefaultServer{
		ctx:       ctx,
		stop:      stop,
		server:    dnsServer,
		dnsMux:    mux,
		dnsMuxMap: make(registrationMap),
		localResolver: &localResolver{
			registeredMap: make(registrationMap),
		},
		wgInterface: wgInterface,
		runtimePort: port,
		runtimeIP:   listenIP,
	}

	hostmanager, err := newHostManager(wgInterface)
	if err != nil {
		return nil, err
	}
	defaultServer.hostManager = hostmanager
	return defaultServer, err
}

// Start runs the listener in a go routine
func (s *DefaultServer) Start() {
	s.runtimePort = port
	udpAddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort(s.server.Addr))
	probeListener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Warnf("using a custom port for dns server")
		s.runtimePort = customPort
		s.server.Addr = fmt.Sprintf("%s:%d", s.runtimeIP, customPort)
	} else {
		err = probeListener.Close()
		if err != nil {
			log.Errorf("got an error closing the probe listener, error: %s", err)
		}
	}

	log.Debugf("starting dns on %s", s.server.Addr)

	go func() {
		s.setListenerStatus(true)
		defer s.setListenerStatus(false)

		err = s.server.ListenAndServe()
		if err != nil {
			log.Errorf("dns server running with %d port returned an error: %v. Will not retry", s.runtimePort, err)
		}
	}()
}

func (s *DefaultServer) setListenerStatus(running bool) {
	s.listenerIsRunning = running
}

// Stop stops the server
func (s *DefaultServer) Stop() {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.stop()

	err := s.hostManager.restoreHostDNS()
	if err != nil {
		log.Error(err)
	}

	err = s.stopListener()
	if err != nil {
		log.Error(err)
	}
}

func (s *DefaultServer) stopListener() error {
	if !s.listenerIsRunning {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.ShutdownContext(ctx)
	if err != nil {
		return fmt.Errorf("stopping dns server listener returned an error: %v", err)
	}
	return nil
}

// UpdateDNSServer processes an update received from the management service
func (s *DefaultServer) UpdateDNSServer(serial uint64, update nbdns.Config) error {
	select {
	case <-s.ctx.Done():
		log.Infof("not updating DNS server as context is closed")
		return s.ctx.Err()
	default:
		if serial < s.updateSerial {
			return fmt.Errorf("not applying dns update, error: "+
				"network update is %d behind the last applied update", s.updateSerial-serial)
		}
		s.mux.Lock()
		defer s.mux.Unlock()

		// is the service should be disabled, we stop the listener
		// and proceed with a regular update to clean up the handlers and records
		if !update.ServiceEnable {
			err := s.stopListener()
			if err != nil {
				log.Error(err)
			}
		} else if !s.listenerIsRunning {
			s.Start()
		}

		localMuxUpdates, localRecords, err := s.buildLocalHandlerUpdate(update.CustomZones)
		if err != nil {
			return fmt.Errorf("not applying dns update, error: %v", err)
		}
		upstreamMuxUpdates, err := s.buildUpstreamHandlerUpdate(update.NameServerGroups)
		if err != nil {
			return fmt.Errorf("not applying dns update, error: %v", err)
		}

		muxUpdates := append(localMuxUpdates, upstreamMuxUpdates...)

		s.updateMux(muxUpdates)
		s.updateLocalResolver(localRecords)

		err = s.hostManager.applyDNSConfig(dnsConfigToHostDNSConfig(update, s.runtimeIP, s.runtimePort))
		if err != nil {
			log.Error(err)
		}

		s.updateSerial = serial

		return nil
	}
}

func (s *DefaultServer) buildLocalHandlerUpdate(customZones []nbdns.CustomZone) ([]muxUpdate, map[string]nbdns.SimpleRecord, error) {
	var muxUpdates []muxUpdate
	localRecords := make(map[string]nbdns.SimpleRecord, 0)

	for _, customZone := range customZones {

		if len(customZone.Records) == 0 {
			return nil, nil, fmt.Errorf("received an empty list of records")
		}

		muxUpdates = append(muxUpdates, muxUpdate{
			domain:  customZone.Domain,
			handler: s.localResolver,
		})

		for _, record := range customZone.Records {
			var class uint16 = dns.ClassINET
			if record.Class != nbdns.DefaultClass {
				return nil, nil, fmt.Errorf("received an invalid class type: %s", record.Class)
			}
			key := buildRecordKey(record.Name, class, uint16(record.Type))
			localRecords[key] = record
		}
	}
	return muxUpdates, localRecords, nil
}

func (s *DefaultServer) buildUpstreamHandlerUpdate(nameServerGroups []*nbdns.NameServerGroup) ([]muxUpdate, error) {
	var muxUpdates []muxUpdate
	for _, nsGroup := range nameServerGroups {
		if len(nsGroup.NameServers) == 0 {
			return nil, fmt.Errorf("received a nameserver group with empty nameserver list")
		}
		handler := &upstreamResolver{
			parentCTX:       s.ctx,
			upstreamClient:  &dns.Client{},
			upstreamTimeout: defaultUpstreamTimeout,
		}
		for _, ns := range nsGroup.NameServers {
			if ns.NSType != nbdns.UDPNameServerType {
				log.Warnf("skiping nameserver %s with type %s, this peer supports only %s",
					ns.IP.String(), ns.NSType.String(), nbdns.UDPNameServerType.String())
				continue
			}
			handler.upstreamServers = append(handler.upstreamServers, getNSHostPort(ns))
		}

		if len(handler.upstreamServers) == 0 {
			log.Errorf("received a nameserver group with an invalid nameserver list")
			continue
		}

		if nsGroup.Primary {
			muxUpdates = append(muxUpdates, muxUpdate{
				domain:  nbdns.RootZone,
				handler: handler,
			})
			continue
		}

		if len(nsGroup.Domains) == 0 {
			return nil, fmt.Errorf("received a non primary nameserver group with an empty domain list")
		}

		for _, domain := range nsGroup.Domains {
			if domain == "" {
				return nil, fmt.Errorf("received a nameserver group with an empty domain element")
			}
			muxUpdates = append(muxUpdates, muxUpdate{
				domain:  domain,
				handler: handler,
			})
		}
	}
	return muxUpdates, nil
}

func (s *DefaultServer) updateMux(muxUpdates []muxUpdate) {
	muxUpdateMap := make(registrationMap)

	for _, update := range muxUpdates {
		s.registerMux(update.domain, update.handler)
		muxUpdateMap[update.domain] = struct{}{}
	}

	for key := range s.dnsMuxMap {
		_, found := muxUpdateMap[key]
		if !found {
			s.deregisterMux(key)
		}
	}

	s.dnsMuxMap = muxUpdateMap
}

func (s *DefaultServer) updateLocalResolver(update map[string]nbdns.SimpleRecord) {
	for key := range s.localResolver.registeredMap {
		_, found := update[key]
		if !found {
			s.localResolver.deleteRecord(key)
		}
	}

	updatedMap := make(registrationMap)
	for key, record := range update {
		err := s.localResolver.registerRecord(record)
		if err != nil {
			log.Warnf("got an error while registering the record (%s), error: %v", record.String(), err)
		}
		updatedMap[key] = struct{}{}
	}

	s.localResolver.registeredMap = updatedMap
}

func getNSHostPort(ns nbdns.NameServer) string {
	return fmt.Sprintf("%s:%d", ns.IP.String(), ns.Port)
}

func (s *DefaultServer) registerMux(pattern string, handler dns.Handler) {
	s.dnsMux.Handle(pattern, handler)
}

func (s *DefaultServer) deregisterMux(pattern string) {
	s.dnsMux.HandleRemove(pattern)
}
