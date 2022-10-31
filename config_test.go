package geoip

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func TestParseConfig(t *testing.T) {
	h := httpcaddyfile.Helper{
		Dispenser: caddyfile.NewTestDispenser(`
		log {
			level DEBUG
			output file /var/log/caddy/test.log
			format json
		}
		geoip /etc/caddy/GeoLite2-City.mmdb
		geoip /path/to/db.mmdb
		header X-Country-Code {geoip_country_code}
		`),
	}
	actual, err := parseCaddyfile(h)
	got := actual.(GeoIP).Config
	if err != nil {
		t.Errorf("parseConfig return err: %v", err)
	}
	expected := Config{
		DatabasePath: "/etc/caddy/GeoLite2-City.mmdb",
	}
	if expected != got {
		t.Errorf("Expected %v got %v", expected, got)
	}
}
