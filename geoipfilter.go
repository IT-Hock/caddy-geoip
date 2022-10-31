package geoip

import (
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/logging"
	"github.com/mmcloughlin/geohash"
	"github.com/oschwald/maxminddb-golang"
	"go.uber.org/zap/zapcore"
	"log"
	"net"
	"strconv"
	"strings"
)

// GeoIP represents a middleware instance
type GeoIPFilter struct {
	DBHandler *maxminddb.Reader
	Config    Config
}

// Interface guards
var (
	_ caddy.Provisioner      = (*GeoIPFilter)(nil)
	_ caddy.Validator        = (*GeoIPFilter)(nil)
	_ caddyfile.Unmarshaler  = (*GeoIPFilter)(nil)
	_ logging.LogFieldFilter = (*GeoIPFilter)(nil)
)

// Init initializes the module
func init() {
	caddy.RegisterModule(GeoIPFilter{})
}

// CaddyModule returns the Caddy module information.
func (GeoIPFilter) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.logging.encoders.filter.geoip",
		New: func() caddy.Module { return new(GeoIPFilter) },
	}
}

// Provision implements caddy.Provisioner.
func (g *GeoIPFilter) Provision(ctx caddy.Context) error {
	dbPath := g.Config.DatabasePath
	if dbPath == "" {
		return fmt.Errorf("a db path is required")
	}
	dbhandler, err := maxminddb.Open(dbPath)
	if err != nil {
		return fmt.Errorf("geoip: Can't open database: " + dbPath)
	}
	g.DBHandler = dbhandler
	return nil
}

// Validate implements caddy.Validator.
func (g *GeoIPFilter) Validate() error {
	if g.DBHandler == nil {
		return fmt.Errorf("no db")
	}
	return nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (g *GeoIPFilter) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if !d.Args(&g.Config.DatabasePath) {
			return d.ArgErr()
		}
	}
	return nil
}

func (g GeoIPFilter) Filter(in zapcore.Field) zapcore.Field {
	request, ok := in.Interface.(caddyhttp.LoggableHTTPRequest)
	if !ok {
		return in
	}
	ip, err := getClientIPFromLoggable(request)
	if err != nil {
		log.Println(err)
		return in
	}
	record, err := g.fetchGeoIPData(ip)
	if err != nil {
		log.Println(err)
		return in
	}
	httpHeader := request.Header
	httpHeader.Add("X-GeoIP-Country-Code", record.Country.ISOCode)
	httpHeader.Add("X-GeoIP-Country-Name", record.Country.Names["en"])
	httpHeader.Add("X-GeoIP-Europe", strconv.FormatBool(record.Country.IsInEuropeanUnion))
	httpHeader.Add("X-GeoIP-Country-GeoNameID", strconv.FormatUint(record.Country.GeoNameID, 10))
	httpHeader.Add("X-GeoIP-City-Name", record.City.Names["en"])
	httpHeader.Add("X-GeoIP-City-GeoNameID", strconv.FormatUint(record.City.GeoNameID, 10))
	httpHeader.Add("X-GeoIP-Latitude", strconv.FormatFloat(record.Location.Latitude, 'f', 6, 64))
	httpHeader.Add("X-GeoIP-Longitude", strconv.FormatFloat(record.Location.Longitude, 'f', 6, 64))
	httpHeader.Add("X-GeoIP-GeoHash", geohash.Encode(record.Location.Latitude, record.Location.Longitude))
	httpHeader.Add("X-GeoIP-TimeZone", record.Location.TimeZone)
	httpHeader.Add("X-GeoIP-Client-IP", record.Client.IP)
	request.Header = httpHeader
	in.Interface = request
	return in
}

func getClientIPFromLoggable(request caddyhttp.LoggableHTTPRequest) (net.IP, error) {
	var ip string

	// Use the client ip from the 'X-Forwarded-For' header, if available.
	if fwdFor := request.Header.Get("X-Forwarded-For"); fwdFor != "" {
		ips := strings.Split(fwdFor, ", ")
		ip = ips[0]
	} else {
		// Otherwise, get the client ip from the request remote address.
		var err error
		ip, _, err = net.SplitHostPort(request.RemoteAddr)
		if err != nil {
			if serr, ok := err.(*net.AddrError); ok && serr.Err == "missing port in address" { // It's not critical try parse
				ip = request.RemoteAddr
			} else {
				log.Printf("Error when SplitHostPort: %v", serr.Err)
				return nil, err
			}
		}
	}

	// Parse the ip address string into a net.IP.
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("unable to parse ip address: %s", ip)
	}

	return parsedIP, nil
}

func (g GeoIPFilter) fetchGeoIPData(ip net.IP) (geoIPRecord, error) {
	var record = geoIPRecord{}
	err := g.DBHandler.Lookup(ip, &record)
	if err != nil {
		return record, err
	}

	record.Client.IP = ip.String()

	if record.Country.ISOCode == "" {
		record.Country.Names = make(map[string]string)
		record.City.Names = make(map[string]string)
		if ip.IsLoopback() {
			record.Country.ISOCode = "**"
			record.Country.Names["en"] = "Loopback"
			record.City.Names["en"] = "Loopback"
		} else {
			record.Country.ISOCode = "!!"
			record.Country.Names["en"] = "No Country"
			record.City.Names["en"] = "No City"
		}
	}
	return record, nil
}
