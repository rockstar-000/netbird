package dns

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/miekg/dns"
	"github.com/mitchellh/hashstructure/v2"
	log "github.com/sirupsen/logrus"

	nbdns "github.com/netbirdio/netbird/dns"
	"github.com/netbirdio/netbird/iface"
)

const (
	defaultPort = 53
	customPort  = 5053
	defaultIP   = "127.0.0.1"
	customIP    = "127.0.0.153"
)

// Server is a dns server interface
type Server interface {
	Start()
	Stop()
	DnsIP() string
	UpdateDNSServer(serial uint64, update nbdns.Config) error
}

type registeredHandlerMap map[string]handlerWithStop

// DefaultServer dns server object
type DefaultServer struct {
	ctx                context.Context
	ctxCancel          context.CancelFunc
	mux                sync.Mutex
	fakeResolverWG     sync.WaitGroup
	server             *dns.Server
	dnsMux             *dns.ServeMux
	dnsMuxMap          registeredHandlerMap
	localResolver      *localResolver
	wgInterface        *iface.WGIface
	hostManager        hostManager
	updateSerial       uint64
	listenerIsRunning  bool
	runtimePort        int
	runtimeIP          string
	previousConfigHash uint64
	currentConfig      hostDNSConfig
	customAddress      *netip.AddrPort
	enabled            bool
}

type handlerWithStop interface {
	dns.Handler
	stop()
}

type muxUpdate struct {
	domain  string
	handler handlerWithStop
}

// NewDefaultServer returns a new dns server
func NewDefaultServer(ctx context.Context, wgInterface *iface.WGIface, customAddress string, initialDnsCfg *nbdns.Config) (*DefaultServer, error) {
	mux := dns.NewServeMux()

	var addrPort *netip.AddrPort
	if customAddress != "" {
		parsedAddrPort, err := netip.ParseAddrPort(customAddress)
		if err != nil {
			return nil, fmt.Errorf("unable to parse the custom dns address, got error: %s", err)
		}
		addrPort = &parsedAddrPort
	}

	hostManager, err := newHostManager(wgInterface)
	if err != nil {
		return nil, err
	}

	ctx, stop := context.WithCancel(ctx)

	defaultServer := &DefaultServer{
		ctx:       ctx,
		ctxCancel: stop,
		server: &dns.Server{
			Net:     "udp",
			Handler: mux,
			UDPSize: 65535,
		},
		dnsMux:    mux,
		dnsMuxMap: make(registeredHandlerMap),
		localResolver: &localResolver{
			registeredMap: make(registrationMap),
		},
		wgInterface:   wgInterface,
		customAddress: addrPort,
		hostManager:   hostManager,
	}

	if initialDnsCfg != nil {
		defaultServer.enabled = hasValidDnsServer(initialDnsCfg)
	}

	defaultServer.evalRuntimeAddress()
	return defaultServer, nil
}

// Start runs the listener in a go routine
func (s *DefaultServer) Start() {
	// nil check required in unit tests
	if s.wgInterface != nil && s.wgInterface.IsUserspaceBind() {
		s.fakeResolverWG.Add(1)
		go func() {
			s.setListenerStatus(true)
			defer s.setListenerStatus(false)

			hookID := s.filterDNSTraffic()
			s.fakeResolverWG.Wait()
			if err := s.wgInterface.GetFilter().RemovePacketHook(hookID); err != nil {
				log.Errorf("unable to remove DNS packet hook: %s", err)
			}
		}()
		return
	}

	log.Debugf("starting dns on %s", s.server.Addr)

	go func() {
		s.setListenerStatus(true)
		defer s.setListenerStatus(false)

		err := s.server.ListenAndServe()
		if err != nil {
			log.Errorf("dns server running with %d port returned an error: %v. Will not retry", s.runtimePort, err)
		}
	}()
}

func (s *DefaultServer) DnsIP() string {
	if !s.enabled {
		return ""
	}
	return s.runtimeIP
}

func (s *DefaultServer) getFirstListenerAvailable() (string, int, error) {
	ips := []string{defaultIP, customIP}
	if runtime.GOOS != "darwin" && s.wgInterface != nil {
		ips = append([]string{s.wgInterface.Address().IP.String()}, ips...)
	}
	ports := []int{defaultPort, customPort}
	for _, port := range ports {
		for _, ip := range ips {
			addrString := fmt.Sprintf("%s:%d", ip, port)
			udpAddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort(addrString))
			probeListener, err := net.ListenUDP("udp", udpAddr)
			if err == nil {
				err = probeListener.Close()
				if err != nil {
					log.Errorf("got an error closing the probe listener, error: %s", err)
				}
				return ip, port, nil
			}
			log.Warnf("binding dns on %s is not available, error: %s", addrString, err)
		}
	}
	return "", 0, fmt.Errorf("unable to find an unused ip and port combination. IPs tested: %v and ports %v", ips, ports)
}

func (s *DefaultServer) setListenerStatus(running bool) {
	s.listenerIsRunning = running
}

// Stop stops the server
func (s *DefaultServer) Stop() {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.ctxCancel()

	err := s.hostManager.restoreHostDNS()
	if err != nil {
		log.Error(err)
	}

	if s.wgInterface != nil && s.wgInterface.IsUserspaceBind() && s.listenerIsRunning {
		s.fakeResolverWG.Done()
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

		hash, err := hashstructure.Hash(update, hashstructure.FormatV2, &hashstructure.HashOptions{
			ZeroNil:         true,
			IgnoreZeroValue: true,
			SlicesAsSets:    true,
			UseStringer:     true,
		})
		if err != nil {
			log.Errorf("unable to hash the dns configuration update, got error: %s", err)
		}

		if s.previousConfigHash == hash {
			log.Debugf("not applying the dns configuration update as there is nothing new")
			s.updateSerial = serial
			return nil
		}

		if err := s.applyConfiguration(update); err != nil {
			return err
		}

		s.updateSerial = serial
		s.previousConfigHash = hash

		return nil
	}
}

func (s *DefaultServer) applyConfiguration(update nbdns.Config) error {
	// is the service should be disabled, we stop the listener or fake resolver
	// and proceed with a regular update to clean up the handlers and records
	if !update.ServiceEnable {
		if s.wgInterface != nil && s.wgInterface.IsUserspaceBind() {
			s.fakeResolverWG.Done()
		} else {
			if err := s.stopListener(); err != nil {
				log.Error(err)
			}
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
	s.currentConfig = dnsConfigToHostDNSConfig(update, s.runtimeIP, s.runtimePort)

	hostUpdate := s.currentConfig
	if s.runtimePort != defaultPort && !s.hostManager.supportCustomPort() {
		log.Warnf("the DNS manager of this peer doesn't support custom port. Disabling primary DNS setup. " +
			"Learn more at: https://netbird.io/docs/how-to-guides/nameservers#local-resolver")
		hostUpdate.routeAll = false
	}

	if err = s.hostManager.applyDNSConfig(hostUpdate); err != nil {
		log.Error(err)
	}

	return nil
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
			log.Warn("received a nameserver group with empty nameserver list")
			continue
		}

		handler := newUpstreamResolver(s.ctx)
		for _, ns := range nsGroup.NameServers {
			if ns.NSType != nbdns.UDPNameServerType {
				log.Warnf("skiping nameserver %s with type %s, this peer supports only %s",
					ns.IP.String(), ns.NSType.String(), nbdns.UDPNameServerType.String())
				continue
			}
			handler.upstreamServers = append(handler.upstreamServers, getNSHostPort(ns))
		}

		if len(handler.upstreamServers) == 0 {
			handler.stop()
			log.Errorf("received a nameserver group with an invalid nameserver list")
			continue
		}

		// when upstream fails to resolve domain several times over all it servers
		// it will calls this hook to exclude self from the configuration and
		// reapply DNS settings, but it not touch the original configuration and serial number
		// because it is temporal deactivation until next try
		//
		// after some period defined by upstream it trys to reactivate self by calling this hook
		// everything we need here is just to re-apply current configuration because it already
		// contains this upstream settings (temporal deactivation not removed it)
		handler.deactivate, handler.reactivate = s.upstreamCallbacks(nsGroup, handler)

		if nsGroup.Primary {
			muxUpdates = append(muxUpdates, muxUpdate{
				domain:  nbdns.RootZone,
				handler: handler,
			})
			continue
		}

		if len(nsGroup.Domains) == 0 {
			handler.stop()
			return nil, fmt.Errorf("received a non primary nameserver group with an empty domain list")
		}

		for _, domain := range nsGroup.Domains {
			if domain == "" {
				handler.stop()
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
	muxUpdateMap := make(registeredHandlerMap)

	for _, update := range muxUpdates {
		s.registerMux(update.domain, update.handler)
		muxUpdateMap[update.domain] = update.handler
		if existingHandler, ok := s.dnsMuxMap[update.domain]; ok {
			existingHandler.stop()
		}
	}

	for key, existingHandler := range s.dnsMuxMap {
		_, found := muxUpdateMap[key]
		if !found {
			existingHandler.stop()
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

// upstreamCallbacks returns two functions, the first one is used to deactivate
// the upstream resolver from the configuration, the second one is used to
// reactivate it. Not allowed to call reactivate before deactivate.
func (s *DefaultServer) upstreamCallbacks(
	nsGroup *nbdns.NameServerGroup,
	handler dns.Handler,
) (deactivate func(), reactivate func()) {
	var removeIndex map[string]int
	deactivate = func() {
		s.mux.Lock()
		defer s.mux.Unlock()

		l := log.WithField("nameservers", nsGroup.NameServers)
		l.Info("temporary deactivate nameservers group due timeout")

		removeIndex = make(map[string]int)
		for _, domain := range nsGroup.Domains {
			removeIndex[domain] = -1
		}
		if nsGroup.Primary {
			removeIndex[nbdns.RootZone] = -1
			s.currentConfig.routeAll = false
		}

		for i, item := range s.currentConfig.domains {
			if _, found := removeIndex[item.domain]; found {
				s.currentConfig.domains[i].disabled = true
				s.deregisterMux(item.domain)
				removeIndex[item.domain] = i
			}
		}
		if err := s.hostManager.applyDNSConfig(s.currentConfig); err != nil {
			l.WithError(err).Error("fail to apply nameserver deactivation on the host")
		}
	}
	reactivate = func() {
		s.mux.Lock()
		defer s.mux.Unlock()

		for domain, i := range removeIndex {
			if i == -1 || i >= len(s.currentConfig.domains) || s.currentConfig.domains[i].domain != domain {
				continue
			}
			s.currentConfig.domains[i].disabled = false
			s.registerMux(domain, handler)
		}

		l := log.WithField("nameservers", nsGroup.NameServers)
		l.Debug("reactivate temporary disabled nameserver group")

		if nsGroup.Primary {
			s.currentConfig.routeAll = true
		}
		if err := s.hostManager.applyDNSConfig(s.currentConfig); err != nil {
			l.WithError(err).Error("reactivate temporary disabled nameserver group, DNS update apply")
		}
	}
	return
}

func (s *DefaultServer) filterDNSTraffic() string {
	filter := s.wgInterface.GetFilter()
	if filter == nil {
		log.Error("can't set DNS filter, filter not initialized")
		return ""
	}

	firstLayerDecoder := layers.LayerTypeIPv4
	if s.wgInterface.Address().Network.IP.To4() == nil {
		firstLayerDecoder = layers.LayerTypeIPv6
	}

	hook := func(packetData []byte) bool {
		// Decode the packet
		packet := gopacket.NewPacket(packetData, firstLayerDecoder, gopacket.Default)

		// Get the UDP layer
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		udp := udpLayer.(*layers.UDP)

		msg := new(dns.Msg)
		if err := msg.Unpack(udp.Payload); err != nil {
			log.Tracef("parse DNS request: %v", err)
			return true
		}

		writer := responseWriter{
			packet: packet,
			device: s.wgInterface.GetDevice().Device,
		}
		go s.dnsMux.ServeDNS(&writer, msg)
		return true
	}

	return filter.AddUDPPacketHook(false, net.ParseIP(s.runtimeIP), uint16(s.runtimePort), hook)
}

func (s *DefaultServer) evalRuntimeAddress() {
	defer func() {
		s.server.Addr = fmt.Sprintf("%s:%d", s.runtimeIP, s.runtimePort)
	}()

	if s.wgInterface != nil && s.wgInterface.IsUserspaceBind() {
		s.runtimeIP = getLastIPFromNetwork(s.wgInterface.Address().Network, 1)
		s.runtimePort = defaultPort
		return
	}

	if s.customAddress != nil {
		s.runtimeIP = s.customAddress.Addr().String()
		s.runtimePort = int(s.customAddress.Port())
		return
	}

	ip, port, err := s.getFirstListenerAvailable()
	if err != nil {
		log.Error(err)
		return
	}
	s.runtimeIP = ip
	s.runtimePort = port
}

func getLastIPFromNetwork(network *net.IPNet, fromEnd int) string {
	// Calculate the last IP in the CIDR range
	var endIP net.IP
	for i := 0; i < len(network.IP); i++ {
		endIP = append(endIP, network.IP[i]|^network.Mask[i])
	}

	// convert to big.Int
	endInt := big.NewInt(0)
	endInt.SetBytes(endIP)

	// subtract fromEnd from the last ip
	fromEndBig := big.NewInt(int64(fromEnd))
	resultInt := big.NewInt(0)
	resultInt.Sub(endInt, fromEndBig)

	return net.IP(resultInt.Bytes()).String()
}

func hasValidDnsServer(cfg *nbdns.Config) bool {
	for _, c := range cfg.NameServerGroups {
		if c.Primary {
			return true
		}
	}
	return false
}
