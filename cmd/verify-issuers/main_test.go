package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	admin    = "issuer::1220aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	registry = "https://example.com/registry"
)

func writeEntry(t *testing.T, root, dir string, mutate func(map[string]any)) {
	t.Helper()
	m := map[string]any{
		"schemaVersion": 1, "network": "testnet", "assetKey": strings.ToUpper(dir),
		"adminParty": admin, "instrumentId": strings.ToUpper(dir),
		"issuerName": "Issuer", "name": strings.ToUpper(dir), "symbol": strings.ToUpper(dir), "decimals": 10,
		"logo": "logo.png", "website": "https://example.com", "registryUrl": registry,
		"wallet": map[string]any{"enabled": true, "visibleByDefault": false},
		"data":   map[string]any{"listed": false}, "pricing": map[string]any{"type": "none"},
	}
	if mutate != nil {
		mutate(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(root, "testnet", dir)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	var logo bytes.Buffer
	if err := png.Encode(&logo, image.NewNRGBA(image.Rect(0, 0, 128, 128))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "info.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "logo.png"), logo.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type instrument struct {
	Symbol, Name string
	Decimals     int
}

type fakeIssuer struct {
	server *httptest.Server
	hits   atomic.Int32
}

func newFakeIssuer(t *testing.T, adminID string, instruments map[string]instrument) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		switch {
		case r.URL.Path == "/registry/metadata/v1/info":
			json.NewEncoder(w).Encode(map[string]any{"adminId": adminID})
		case strings.HasPrefix(r.URL.Path, "/registry/metadata/v1/instruments/"):
			id := strings.TrimPrefix(r.URL.Path, "/registry/metadata/v1/instruments/")
			switch id {
			case "MOVED":
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
				return
			case "HTML":
				w.Write([]byte("<html>not metadata</html>"))
				return
			}
			ins, ok := instruments[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"id": id, "symbol": ins.Symbol, "name": ins.Name, "decimals": ins.Decimals})
		case r.URL.Path == "/elsewhere":
			json.NewEncoder(w).Encode(map[string]any{"decimals": 10, "symbol": "MOVED", "name": "MOVED"})
		case strings.HasPrefix(r.URL.Path, "/api/v3/coins/"):
			switch strings.TrimPrefix(r.URL.Path, "/api/v3/coins/") {
			case "beth-token":
				json.NewEncoder(w).Encode(map[string]any{"id": "beth-token", "symbol": "beth", "name": "BETH"})
			case "busy":
				w.WriteHeader(http.StatusTooManyRequests)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// The test server's certificate covers example.com, so entries can use a public-looking host.
func (f *fakeIssuer) verifier() *verifier {
	client := f.server.Client()
	transport := client.Transport.(*http.Transport).Clone()
	addr := f.server.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	client.Transport = transport
	return newVerifier(client, "https://example.com/api/v3", 0)
}

func summaryOf(t *testing.T, head, base string, limit int, v *verifier) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "summary.md")
	err := run(head, base, path, limit, v)
	data, _ := os.ReadFile(path)
	return string(data), err
}

func row(summary, entry, result string) bool {
	return regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta(entry) + ` \| [^|]+ \| ` + result + ` \|`).MatchString(summary)
}

func TestVerifiesNewEntriesAgainstTheRegistry(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, map[string]instrument{
		"GOOD": {"GOOD", "GOOD", 10},
		"DEC":  {"DEC", "DEC", 6},
		"SYM":  {"SYMX", "SYM", 10},
	})
	writeEntry(t, base, "good", nil)
	writeEntry(t, head, "good", nil)
	for _, dir := range []string{"dec", "sym", "missing"} {
		writeEntry(t, head, dir, nil)
	}
	out, err := summaryOf(t, head, base, 20, issuer.verifier())
	if err == nil || err.Error() != "2 mismatches with issuer data" {
		t.Fatalf("want 2 mismatches (decimals, missing instrument), got %v", err)
	}
	if !row(out, "testnet/dec/info.json", "mismatch") || !row(out, "testnet/missing/info.json", "mismatch") {
		t.Errorf("mismatch rows missing:\n%s", out)
	}
	if !row(out, "testnet/sym/info.json", "warning") || row(out, "testnet/sym/info.json", "mismatch") {
		t.Errorf("a symbol difference must only warn:\n%s", out)
	}
	if strings.Contains(out, "testnet/good/info.json") {
		t.Errorf("unchanged entry was verified:\n%s", out)
	}
}

func TestAdminMismatch(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, "someone-else::1220bb", map[string]instrument{"NEW": {"NEW", "NEW", 10}})
	writeEntry(t, head, "new", nil)
	if _, err := summaryOf(t, head, base, 20, issuer.verifier()); err == nil || err.Error() != "1 mismatch with issuer data" {
		t.Fatalf("want an adminId mismatch, got %v", err)
	}
}

func TestDoesNotFollowRedirects(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, nil)
	writeEntry(t, head, "moved", nil)
	out, err := summaryOf(t, head, base, 20, issuer.verifier())
	if err != nil {
		t.Fatalf("a redirect is a warning, got %v", err)
	}
	if !strings.Contains(out, "HTTP 302") || strings.Contains(out, "decimals match") {
		t.Fatalf("redirect was followed:\n%s", out)
	}
}

func TestNonJSONMetadataIsAMismatch(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, nil)
	writeEntry(t, head, "html", nil)
	if _, err := summaryOf(t, head, base, 20, issuer.verifier()); err == nil || err.Error() != "1 mismatch with issuer data" {
		t.Fatalf("want a mismatch for a 200 that is not JSON, got %v", err)
	}
}

func TestSkipsEntriesThatFailTheRequiredChecks(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, nil)
	writeEntry(t, head, "local", func(m map[string]any) { m["registryUrl"] = "https://registry.internal/registry" })
	out, err := summaryOf(t, head, base, 20, issuer.verifier())
	if err != nil {
		t.Fatal(err)
	}
	if issuer.hits.Load() != 0 || !strings.Contains(out, "| testnet/local/info.json | skipped | warning |") || strings.Contains(out, "| testnet/local/info.json | registry info |") {
		t.Fatalf("called %d URLs for an entry that fails Validate assets:\n%s", issuer.hits.Load(), out)
	}
}

func TestCoinGeckoSources(t *testing.T) {
	issuer := newFakeIssuer(t, admin, map[string]instrument{"NEW": {"NEW", "NEW", 10}})
	for _, tc := range []struct {
		id       string
		mismatch bool
	}{
		{"beth-token", false},
		{"no-such-coin", true},
		{"busy", false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			base, head := t.TempDir(), t.TempDir()
			writeEntry(t, head, "new", func(m map[string]any) {
				m["pricing"] = map[string]any{"type": "reference", "sources": []any{
					map[string]any{"provider": "coingecko", "providerAssetId": tc.id},
				}}
			})
			_, err := summaryOf(t, head, base, 20, issuer.verifier())
			if (err != nil) != tc.mismatch {
				t.Fatalf("mismatch=%v, err=%v", tc.mismatch, err)
			}
		})
	}
}

func TestNetworkErrorsAreWarnings(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, nil)
	v := issuer.verifier()
	issuer.server.Close()
	writeEntry(t, head, "new", nil)
	out, err := summaryOf(t, head, base, 20, v)
	if err != nil {
		t.Fatalf("network errors must not fail the job: %v", err)
	}
	if !row(out, "testnet/new/info.json", "warning") {
		t.Fatalf("summary misses the warning:\n%s", out)
	}
}

func TestFailsWhenEntriesGoUnchecked(t *testing.T) {
	base, head := t.TempDir(), t.TempDir()
	issuer := newFakeIssuer(t, admin, map[string]instrument{"A": {"A", "A", 10}, "B": {"B", "B", 10}, "C": {"C", "C", 10}})
	for _, dir := range []string{"a", "b", "c"} {
		writeEntry(t, head, dir, nil)
	}
	out, err := summaryOf(t, head, base, 2, issuer.verifier())
	if err == nil || !strings.Contains(err.Error(), "1 entries over the limit") {
		t.Fatalf("want a failure for the unchecked entry, got %v", err)
	}
	if strings.Contains(out, "testnet/c/info.json") || !strings.Contains(out, "1 more entries") {
		t.Fatalf("limit not applied:\n%s", out)
	}
}
