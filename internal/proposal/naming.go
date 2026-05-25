package proposal

import (
	"regexp"
	"strings"
)

const maxDNSSubdomainLength = 253

var nonDNSSafe = regexp.MustCompile(`[^a-z0-9\-.]`)

func ProposalName(alertname, namespace, fingerprint string) string {
	fp := fingerprint
	if len(fp) > 8 {
		fp = fp[:8]
	}
	raw := strings.ToLower(alertname + "-" + namespace + "-" + fp)
	return SanitizeDNSSubdomain(raw)
}

func SanitizeDNSSubdomain(name string) string {
	if name == "" {
		return ""
	}
	s := strings.ToLower(name)
	s = nonDNSSafe.ReplaceAllString(s, "-")
	if len(s) > maxDNSSubdomainLength {
		s = s[:maxDNSSubdomainLength]
	}
	s = strings.Trim(s, "-")
	return s
}
