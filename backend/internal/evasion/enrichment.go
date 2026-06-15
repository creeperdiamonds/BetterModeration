package evasion

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	ipAPIURL    = "http://ip-api.com/json/%s?fields=status,countryCode,isp,org,as,proxy,hosting"
	ipCacheTTL  = 7 * 24 * time.Hour
	ipFetchTimeout = 3 * time.Second
)

// IPMeta holds enrichment data for an IP address.
type IPMeta struct {
	IsVPN     bool
	IsProxy   bool
	IsHosting bool
	ASN       string
	Country   string
	ISP       string
}

// IsPrivateIP returns true for loopback, link-local, and RFC-1918 addresses.
// Private IPs get score 0 for all VPN/ASN signals and skip enrichment entirely.
func IsPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true // treat unparseable as private (safe default)
	}
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}

// GetOrFetchMeta returns cached IP metadata or nil if not yet available.
// When the cache is missing or stale it fires fetchAndCache in a goroutine and
// returns nil immediately — the join path is never blocked on ip-api.com.
// Accurate enrichment is applied on the player's second join at the latest.
func GetOrFetchMeta(ctx context.Context, db *sqlx.DB, ip string) (*IPMeta, error) {
	if IsPrivateIP(ip) {
		return nil, nil
	}

	var row struct {
		CountryCode string    `db:"country_code"`
		ISP         string    `db:"isp"`
		ASN         string    `db:"asn"`
		IsVPN       bool      `db:"is_vpn"`
		IsProxy     bool      `db:"is_proxy"`
		IsHosting   bool      `db:"is_hosting"`
		CachedAt    time.Time `db:"cached_at"`
	}

	err := db.QueryRowxContext(ctx, `
		SELECT country_code, isp, asn, is_vpn, is_proxy, is_hosting, cached_at
		FROM ip_metadata WHERE ip_address = ?`, ip).StructScan(&row)

	if err == nil && time.Since(row.CachedAt) < ipCacheTTL {
		return &IPMeta{
			IsVPN:     row.IsVPN,
			IsProxy:   row.IsProxy,
			IsHosting: row.IsHosting,
			ASN:       row.ASN,
			Country:   row.CountryCode,
			ISP:       row.ISP,
		}, nil
	}

	// Cache miss or stale — fetch asynchronously, return nil now.
	go fetchAndCache(db, ip)
	return nil, nil
}

type ipAPIResponse struct {
	Status      string `json:"status"`
	CountryCode string `json:"countryCode"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
	AS          string `json:"as"`
	Proxy       bool   `json:"proxy"`
	Hosting     bool   `json:"hosting"`
}

func fetchAndCache(db *sqlx.DB, ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), ipFetchTimeout)
	defer cancel()

	client := &http.Client{Timeout: ipFetchTimeout}
	resp, err := client.Get(fmt.Sprintf(ipAPIURL, ip))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var data ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}
	if data.Status != "success" {
		return
	}

	// Extract ASN number from the "AS12345 Name" string.
	asn := ""
	if len(data.AS) > 0 {
		var asnNum int
		fmt.Sscanf(data.AS, "AS%d", &asnNum)
		if asnNum > 0 {
			asn = fmt.Sprintf("AS%d", asnNum)
		}
	}

	db.ExecContext(ctx, `
		INSERT INTO ip_metadata
			(ip_address, country_code, isp, org, asn, is_vpn, is_proxy, is_tor, is_hosting, cached_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, 0, ?, NOW(3))
		ON DUPLICATE KEY UPDATE
			country_code = VALUES(country_code),
			isp          = VALUES(isp),
			org          = VALUES(org),
			asn          = VALUES(asn),
			is_vpn       = VALUES(is_vpn),
			is_proxy     = VALUES(is_proxy),
			is_hosting   = VALUES(is_hosting),
			cached_at    = NOW(3)`,
		ip, data.CountryCode, data.ISP, data.Org, asn,
		data.Proxy, data.Hosting,
	)
}
