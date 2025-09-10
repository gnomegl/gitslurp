package domains

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type WhoisInfo struct {
	Domain         string
	Registrar      string
	CreatedDate    time.Time
	UpdatedDate    time.Time
	ExpiresDate    time.Time
	NameServers    []string
	Status         []string
	Emails         []string
	Organization   string
	Registrant     string
	Raw            string
}

type WhoisClient struct {
	timeout time.Duration
}

func NewWhoisClient() *WhoisClient {
	return &WhoisClient{
		timeout: 10 * time.Second,
	}
}

func (w *WhoisClient) Lookup(ctx context.Context, domain string) (*WhoisInfo, error) {
	server := w.getWhoisServer(domain)
	if server == "" {
		server = "whois.iana.org"
	}

	raw, err := w.query(ctx, server, domain)
	if err != nil {
		return nil, err
	}

	referralServer := w.extractReferralServer(raw)
	if referralServer != "" && referralServer != server {
		referralRaw, err := w.query(ctx, referralServer, domain)
		if err == nil {
			raw = referralRaw
		}
	}

	info := w.parseWhoisResponse(domain, raw)
	return info, nil
}

func (w *WhoisClient) query(ctx context.Context, server, domain string) (string, error) {
	d := net.Dialer{Timeout: w.timeout}
	conn, err := d.DialContext(ctx, "tcp", server+":43")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\r\n", domain)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(conn)
	var response strings.Builder
	for scanner.Scan() {
		response.WriteString(scanner.Text() + "\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return response.String(), nil
}

func (w *WhoisClient) getWhoisServer(domain string) string {
	tld := ""
	parts := strings.Split(domain, ".")
	if len(parts) > 0 {
		tld = parts[len(parts)-1]
	}

	servers := map[string]string{
		"com":  "whois.verisign-grs.com",
		"net":  "whois.verisign-grs.com",
		"org":  "whois.pir.org",
		"info": "whois.afilias.net",
		"io":   "whois.nic.io",
		"co":   "whois.nic.co",
		"uk":   "whois.nic.uk",
		"de":   "whois.denic.de",
		"fr":   "whois.nic.fr",
		"ru":   "whois.tcinet.ru",
		"jp":   "whois.jprs.jp",
	}

	if server, exists := servers[tld]; exists {
		return server
	}

	return ""
}

func (w *WhoisClient) extractReferralServer(response string) string {
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(line, "whois server:") ||
			strings.HasPrefix(line, "registrar whois server:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func (w *WhoisClient) parseWhoisResponse(domain, raw string) *WhoisInfo {
	info := &WhoisInfo{
		Domain:      domain,
		Raw:         raw,
		NameServers: make([]string, 0),
		Status:      make([]string, 0),
		Emails:      make([]string, 0),
	}

	lines := strings.Split(raw, "\n")
	emailMap := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		if strings.Contains(lower, "registrar:") && !strings.Contains(lower, "whois") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.Registrar = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(lower, "creation date:") || strings.Contains(lower, "created:") {
			if date := w.extractDate(line); !date.IsZero() {
				info.CreatedDate = date
			}
		}

		if strings.Contains(lower, "updated date:") || strings.Contains(lower, "last update:") {
			if date := w.extractDate(line); !date.IsZero() {
				info.UpdatedDate = date
			}
		}

		if strings.Contains(lower, "expiry date:") || strings.Contains(lower, "expires:") {
			if date := w.extractDate(line); !date.IsZero() {
				info.ExpiresDate = date
			}
		}

		if strings.Contains(lower, "name server:") || strings.Contains(lower, "nserver:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ns := strings.ToLower(parts[len(parts)-1])
				info.NameServers = append(info.NameServers, ns)
			}
		}

		if strings.Contains(lower, "status:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				status := strings.TrimSpace(parts[1])
				info.Status = append(info.Status, status)
			}
		}

		if strings.Contains(lower, "registrant organization:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.Organization = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(lower, "registrant name:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.Registrant = strings.TrimSpace(parts[1])
			}
		}

		if emails := w.extractEmails(line); len(emails) > 0 {
			for _, email := range emails {
				if !emailMap[email] {
					emailMap[email] = true
					info.Emails = append(info.Emails, email)
				}
			}
		}
	}

	return info
}

func (w *WhoisClient) extractDate(line string) time.Time {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return time.Time{}
	}

	dateStr := strings.TrimSpace(parts[1])
	
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"2006.01.02",
		"02/01/2006",
		"2006-01-02T15:04:05.000Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

func (w *WhoisClient) extractEmails(line string) []string {
	var emails []string
	
	words := strings.Fields(line)
	for _, word := range words {
		if strings.Contains(word, "@") && strings.Contains(word, ".") {
			word = strings.Trim(word, "()<>[]{}\"',;:")
			if strings.Count(word, "@") == 1 {
				emails = append(emails, strings.ToLower(word))
			}
		}
	}

	return emails
}

type DomainInvestigator struct {
	whoisClient *WhoisClient
	finder      *Finder
}

func NewDomainInvestigator() *DomainInvestigator {
	return &DomainInvestigator{
		whoisClient: NewWhoisClient(),
		finder:      NewFinder(),
	}
}

func (d *DomainInvestigator) Investigate(ctx context.Context, domain string) (*DomainInvestigation, error) {
	investigation := &DomainInvestigation{
		Domain:      domain,
		Timestamp:   time.Now(),
	}

	whoisInfo, err := d.whoisClient.Lookup(ctx, domain)
	if err == nil {
		investigation.WhoisInfo = whoisInfo
	}

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ips, _ := d.finder.resolver.LookupIPAddr(ctx2, domain)
	for _, ip := range ips {
		investigation.IPs = append(investigation.IPs, ip.IP.String())
	}

	mxRecords, _ := d.finder.resolver.LookupMX(ctx2, domain)
	for _, mx := range mxRecords {
		investigation.MXRecords = append(investigation.MXRecords, mx.Host)
	}

	txtRecords, _ := d.finder.resolver.LookupTXT(ctx2, domain)
	investigation.TXTRecords = txtRecords

	cname, _ := d.finder.resolver.LookupCNAME(ctx2, domain)
	if cname != "" && cname != domain {
		investigation.CNAME = cname
	}

	investigation.HTTPSEnabled = d.finder.checkSiteAccessible(ctx, "https://"+domain)

	return investigation, nil
}

type DomainInvestigation struct {
	Domain       string
	WhoisInfo    *WhoisInfo
	IPs          []string
	MXRecords    []string
	TXTRecords   []string
	CNAME        string
	HTTPSEnabled bool
	Timestamp    time.Time
}