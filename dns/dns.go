// Package dns implement dns types and standard methods and functions
// to parse and normalize dns records and configuration
package dns

import (
	"fmt"
	"github.com/miekg/dns"
)

const (
	// DefaultDNSPort well-known port number
	DefaultDNSPort = 53
	// RootZone is a string representation of the root zone
	RootZone = "."
	// DefaultClass is the class supported by the system
	DefaultClass = "IN"
)

// Update represents a dns update that is exchanged between management and peers
type Update struct {
	// ServiceEnable indicates if the service should be enabled
	ServiceEnable bool
	// NameServerGroups contains a list of nameserver group
	NameServerGroups []NameServerGroup
	// CustomZones contains a list of custom zone
	CustomZones []CustomZone
}

// CustomZone represents a custom zone to be resolved by the dns server
type CustomZone struct {
	// Domain is the zone's domain
	Domain string
	// Records custom zone records
	Records []SimpleRecord
}

// SimpleRecord provides a simple DNS record specification for CNAME, A and AAAA records
type SimpleRecord struct {
	// Name domain name
	Name string
	// Type of record, 1 for A, 5 for CNAME, 28 for AAAA. see https://pkg.go.dev/github.com/miekg/dns@v1.1.41#pkg-constants
	Type int
	// Class dns class, currently use the DefaultClass for all records
	Class string
	// TTL time-to-live for the record
	TTL int
	// RData is the actual value resolved in a dns query
	RData string
}

// String returns a string of the simple record formatted as:
// <Name> <TTL> <Class> <Type> <RDATA>
func (s SimpleRecord) String() string {
	fqdn := dns.Fqdn(s.Name)
	return fmt.Sprintf("%s %d %s %s %s", fqdn, s.TTL, s.Class, dns.Type(s.Type).String(), s.RData)
}
