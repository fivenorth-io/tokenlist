package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fivenorth-io/tokenlist/submission"
)

func main() {
	root := flag.String("root", ".", "tokenlist checkout to verify")
	base := flag.String("base", "", "base checkout; only entries added or changed since it are verified")
	summary := flag.String("summary", "", "file to append the Markdown summary to, such as $GITHUB_STEP_SUMMARY")
	limit := flag.Int("max", 20, "most entries to verify in one run")
	flag.Parse()
	v := newVerifier(&http.Client{Timeout: 10 * time.Second}, "https://api.coingecko.com/api/v3", 1500*time.Millisecond)
	if err := run(*root, *base, *summary, *limit, v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type status string

const (
	ok       status = "ok"
	mismatch status = "mismatch"
	warning  status = "warning"
)

type result struct {
	path, check string
	status      status
	detail      string
}

type entry struct {
	AdminParty   string `json:"adminParty"`
	InstrumentID string `json:"instrumentId"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Decimals     int64  `json:"decimals"`
	RegistryURL  string `json:"registryUrl"`
	Pricing      struct {
		Type    string `json:"type"`
		Sources []struct {
			Provider        string `json:"provider"`
			ProviderAssetID string `json:"providerAssetId"`
		} `json:"sources"`
	} `json:"pricing"`
}

func run(root, base, summary string, limit int, v *verifier) error {
	if base == "" {
		return errors.New("-base is required")
	}
	report, err := submission.Check(os.DirFS(root), os.DirFS(base))
	if err != nil {
		return err
	}
	invalid := map[string]bool{}
	for _, f := range report.Errors {
		invalid[f.Path] = true
	}
	var paths []string
	for _, a := range report.Added {
		paths = append(paths, a.Path)
	}
	for _, p := range report.Modified {
		if path.Base(p) == "info.json" {
			paths = append(paths, p)
		}
	}
	var results []result
	checked, unchecked := 0, 0
	for _, p := range paths {
		if invalid[p] {
			results = append(results, result{p, "skipped", warning, "fails Validate assets, so its URLs are not called"})
			continue
		}
		if checked == limit {
			unchecked++
			continue
		}
		checked++
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		var e entry
		if err := json.Unmarshal(data, &e); err != nil {
			results = append(results, result{p, "metadata", mismatch, err.Error()})
			continue
		}
		results = append(results, v.verify(p, e)...)
	}
	if summary != "" {
		if err := appendFile(summary, markdown(results, unchecked)); err != nil {
			return err
		}
	}
	failed := 0
	for _, r := range results {
		if r.status != ok {
			fmt.Fprintf(os.Stderr, "%s: %q %s: %s\n", r.status, r.path, r.check, strings.Join(strings.Fields(r.detail), " "))
		}
		if r.status == mismatch {
			failed++
		}
	}
	switch {
	case failed > 0:
		plural := "es"
		if failed == 1 {
			plural = ""
		}
		return fmt.Errorf("%d mismatch%s with issuer data", failed, plural)
	case unchecked > 0:
		return fmt.Errorf("%d entries over the limit of %d were not verified", unchecked, limit)
	}
	fmt.Printf("Verified %d entries against issuer data\n", checked)
	return nil
}

type verifier struct {
	client    *http.Client
	coinGecko string
	gap       time.Duration
	last      time.Time
	admins    map[string]result
}

func newVerifier(client *http.Client, coinGecko string, gap time.Duration) *verifier {
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &verifier{client: client, coinGecko: coinGecko, gap: gap, admins: map[string]result{}}
}

func (v *verifier) verify(p string, e entry) []result {
	registry := strings.TrimSuffix(e.RegistryURL, "/")
	var out []result
	add := func(check string, s status, format string, args ...any) {
		out = append(out, result{p, check, s, fmt.Sprintf(format, args...)})
	}

	cacheKey := registry + "\x00" + e.AdminParty
	admin, seen := v.admins[cacheKey]
	if !seen {
		var info struct {
			AdminID string `json:"adminId"`
		}
		admin = result{check: "registry info"}
		switch code, err := v.get(registry+"/metadata/v1/info", &info); {
		case code == http.StatusOK && err != nil:
			admin.status, admin.detail = mismatch, "registry /metadata/v1/info is not token metadata JSON"
		case err != nil:
			admin.status, admin.detail = warning, err.Error()
		case code == http.StatusNotFound:
			admin.status, admin.detail = mismatch, "registry does not serve /metadata/v1/info"
		case code != http.StatusOK:
			admin.status, admin.detail = warning, fmt.Sprintf("HTTP %d", code)
		case info.AdminID != e.AdminParty:
			admin.status, admin.detail = mismatch, fmt.Sprintf("registry adminId %q is not adminParty", info.AdminID)
		default:
			admin.status, admin.detail = ok, "adminId matches"
		}
		v.admins[cacheKey] = admin
	}
	add(admin.check, admin.status, "%s", admin.detail)

	var ins struct {
		Symbol   string `json:"symbol"`
		Name     string `json:"name"`
		Decimals *int64 `json:"decimals"`
	}
	switch code, err := v.get(registry+"/metadata/v1/instruments/"+url.PathEscape(e.InstrumentID), &ins); {
	case code == http.StatusOK && err != nil:
		add("instrument", mismatch, "registry instrument response is not token metadata JSON")
	case err != nil:
		add("instrument", warning, "%v", err)
	case code == http.StatusNotFound:
		add("instrument", mismatch, "registry has no instrument %q", e.InstrumentID)
	case code != http.StatusOK:
		add("instrument", warning, "HTTP %d", code)
	case ins.Decimals == nil || *ins.Decimals != e.Decimals:
		add("instrument", mismatch, "registry decimals %v, entry decimals %d", deref(ins.Decimals), e.Decimals)
	default:
		add("instrument", ok, "decimals match")
		if ins.Symbol != e.Symbol || ins.Name != e.Name {
			add("instrument", warning, "registry shows symbol %q name %q, entry has %q %q", ins.Symbol, ins.Name, e.Symbol, e.Name)
		}
	}

	for _, s := range e.Pricing.Sources {
		if s.Provider != "coingecko" || v.coinGecko == "" {
			continue
		}
		var coin struct {
			Symbol string `json:"symbol"`
		}
		u := v.coinGecko + "/coins/" + url.PathEscape(s.ProviderAssetID) +
			"?localization=false&tickers=false&market_data=false&community_data=false&developer_data=false&sparkline=false"
		switch code, err := v.get(u, &coin); {
		case err != nil:
			add("coingecko", warning, "%v", err)
		case code == http.StatusNotFound:
			add("coingecko", mismatch, "CoinGecko has no id %q", s.ProviderAssetID)
		case code != http.StatusOK:
			add("coingecko", warning, "HTTP %d", code)
		case e.Pricing.Type == "direct" && !strings.EqualFold(coin.Symbol, e.Symbol):
			add("coingecko", warning, "CoinGecko %q has symbol %q, entry has %q", s.ProviderAssetID, coin.Symbol, e.Symbol)
		default:
			add("coingecko", ok, "id %q exists", s.ProviderAssetID)
		}
	}
	return out
}

func (v *verifier) get(u string, into any) (int, error) {
	if wait := v.gap - time.Since(v.last); wait > 0 {
		time.Sleep(wait)
	}
	defer func() { v.last = time.Now() }()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into); err != nil {
		return resp.StatusCode, fmt.Errorf("unreadable response: %w", err)
	}
	return resp.StatusCode, nil
}

func deref(p *int64) any {
	if p == nil {
		return "missing"
	}
	return *p
}

func markdown(results []result, unchecked int) string {
	var b strings.Builder
	b.WriteString("## Issuer data check (advisory)\n\n")
	if len(results) == 0 {
		b.WriteString("No added or changed entries.\n\n")
	} else {
		b.WriteString("| Entry | Check | Result | Detail |\n|---|---|---|---|\n")
		for _, r := range results {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", submission.EscapeUntrustedCell(r.path), r.check, r.status, submission.EscapeUntrustedCell(r.detail))
		}
		b.WriteString("\n")
	}
	if unchecked > 0 {
		fmt.Fprintf(&b, "%d more entries were not checked in this run.\n\n", unchecked)
	}
	return b.String()
}

func appendFile(name, text string) error {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}
