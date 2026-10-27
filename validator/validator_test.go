package validator

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func metadata(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"schemaVersion": 1, "network": "testnet", "assetKey": "BETH",
		"adminParty": "issuer::1220" + strings.Repeat("a", 64), "instrumentId": "BETH",
		"issuerName": "BitSafe", "name": "BETH", "symbol": "BETH", "decimals": 10,
		"logo": "assets/logo.png", "website": "https://example.com", "registryUrl": "https://example.com/registry",
		"wallet": map[string]any{"enabled": true, "visibleByDefault": true},
		"data":   map[string]any{"listed": true}, "pricing": map[string]any{"type": "none"},
	}
	if mutate != nil {
		mutate(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMetadata(t *testing.T) {
	v := newValidator(t)
	for _, network := range []string{"mainnet", "devnet", "testnet"} {
		for _, kind := range []string{"none", "fixed", "direct", "reference"} {
			t.Run(network+"/"+kind, func(t *testing.T) {
				data := metadata(t, func(m map[string]any) {
					m["network"] = network
					p := map[string]any{"type": kind}
					m["pricing"] = p
					if kind == "fixed" {
						p["currency"], p["value"] = "USD", "1"
					}
					if kind == "direct" || kind == "reference" {
						p["sources"] = []any{map[string]any{"provider": "coinmarketcap", "providerAssetId": "5426"}, map[string]any{"provider": "pyth", "providerAssetId": "feed", "multiplier": "0.01"}}
					}
				})
				if _, err := v.Validate(network+"/beth/info.json", data); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestInvalidMetadata(t *testing.T) {
	v := newValidator(t)
	cases := map[string]func(map[string]any){
		"unknown field":       func(m map[string]any) { m["typo"] = true },
		"missing field":       func(m map[string]any) { delete(m, "assetKey") },
		"wrong network":       func(m map[string]any) { m["network"] = "mainnet" },
		"null decimals":       func(m map[string]any) { m["decimals"] = nil },
		"fractional decimals": func(m map[string]any) { m["decimals"] = 1.5 },
		"invalid admin":       func(m map[string]any) { m["adminParty"] = "issuer" },
		"unsafe logo":         func(m map[string]any) { m["logo"] = "../logo.png" },
		"encoded logo":        func(m map[string]any) { m["logo"] = "%2e%2e/logo.png" },
		"logo extension":      func(m map[string]any) { m["logo"] = "logo.html" },
		"invalid URL":         func(m map[string]any) { m["website"] = "http://example.com" },
		"URL credentials":     func(m map[string]any) { m["registryUrl"] = "https://user:secret@example.com" },
		"comma key":           func(m map[string]any) { m["assetKey"] = "A,B" },
		"whitespace name":     func(m map[string]any) { m["name"] = " BETH " },
		"disabled visible":    func(m map[string]any) { m["wallet"].(map[string]any)["enabled"] = false },
		"empty sources":       func(m map[string]any) { m["pricing"] = map[string]any{"type": "direct", "sources": []any{}} },
		"fixed fallback": func(m map[string]any) {
			m["pricing"] = map[string]any{"type": "reference", "sources": []any{map[string]any{"type": "fixed", "value": "1"}}}
		},
		"numeric value": func(m map[string]any) { m["pricing"] = map[string]any{"type": "fixed", "currency": "USD", "value": 1} },
		"none sources":  func(m map[string]any) { m["pricing"].(map[string]any)["sources"] = []any{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Validate("testnet/beth/info.json", metadata(t, mutate)); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
	for _, raw := range []string{`{`, `{} {}`, strings.Repeat(" ", 65537)} {
		if _, err := v.Validate("testnet/beth/info.json", []byte(raw)); err == nil {
			t.Fatal("accepted invalid JSON")
		}
	}
}

func TestRepository(t *testing.T) {
	v := newValidator(t)
	root := fstest.MapFS{
		"testnet/beth/info.json":         {Data: metadata(t, nil)},
		"testnet/beth/assets/logo.png":   {Data: []byte("logo")},
		"mainnet/legacy/assets/logo.png": {Data: []byte("legacy")},
	}
	entries, err := v.ValidateRepository(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%v: %v", entries, err)
	}
	delete(root, "testnet/beth/assets/logo.png")
	if _, err := v.ValidateRepository(root); err == nil {
		t.Fatal("accepted missing logo")
	}
	if _, err := v.ValidateRepository(fstest.MapFS{}); err != nil {
		t.Fatal("legacy-only migration must be allowed", err)
	}
}

func TestSetAndTransition(t *testing.T) {
	v := newValidator(t)
	a, err := v.Validate("testnet/beth/info.json", metadata(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	b := a
	b.Path = "testnet/second/info.json"
	if ValidateSet([]Entry{a, b}) == nil {
		t.Fatal("accepted duplicate identity")
	}
	b.InstrumentID = "second"
	if ValidateSet([]Entry{a, b}) == nil {
		t.Fatal("accepted duplicate key")
	}
	b.AssetKey = "second"
	b.IssuerName = "Other"
	if ValidateSet([]Entry{a, b}) == nil {
		t.Fatal("accepted issuer mismatch")
	}
	b.IssuerName = a.IssuerName
	b.RegistryURL = "https://different.com"
	if ValidateSet([]Entry{a, b}) == nil {
		t.Fatal("accepted registry mismatch")
	}
	b = a
	b.Network = "mainnet"
	b.Path = "mainnet/beth/info.json"
	if err := ValidateSet([]Entry{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition([]Entry{a}, []Entry{a, b}); err != nil {
		t.Fatal(err)
	}
	if ValidateTransition([]Entry{a}, nil) == nil {
		t.Fatal("accepted deletion")
	}
	b = a
	b.AssetKey = "renamed"
	if ValidateTransition([]Entry{a}, []Entry{b}) == nil {
		t.Fatal("accepted key rename")
	}
	b = a
	b.AdminParty = "other"
	if ValidateTransition([]Entry{a}, []Entry{b}) == nil {
		t.Fatal("accepted identity change")
	}
	b = a
	b.Path = "testnet/new-folder/info.json"
	if err := ValidateTransition([]Entry{a}, []Entry{b}); err != nil {
		t.Fatal("folder is not public identity", err)
	}
}

func TestIntegerEncoding(t *testing.T) {
	v := newValidator(t)
	for _, field := range []string{"schemaVersion", "decimals"} {
		for _, raw := range []string{"1.0", "1e0", "1.00000000000000001"} {
			t.Run(field+"/"+raw, func(t *testing.T) {
				data := metadata(t, func(m map[string]any) { m[field] = json.RawMessage(raw) })
				if _, err := v.Validate("testnet/beth/info.json", data); err == nil {
					t.Fatal("accepted a numeric encoding that Wallet cannot decode")
				}
			})
		}
	}
}

func TestLogoSize(t *testing.T) {
	v := newValidator(t)
	for _, size := range []int{maxLogoBytes, maxLogoBytes + 1} {
		for _, referenced := range []bool{false, true} {
			root := fstest.MapFS{"testnet/beth/assets/logo.png": {Data: make([]byte, size)}}
			if referenced {
				root["testnet/beth/info.json"] = &fstest.MapFile{Data: metadata(t, nil)}
			}
			_, err := v.ValidateRepository(root)
			if (err != nil) != (size > maxLogoBytes) {
				t.Fatalf("size=%d referenced=%v: %v", size, referenced, err)
			}
		}
	}
}

func TestPricingMultiplier(t *testing.T) {
	v := newValidator(t)
	for _, value := range []any{"0", "-1", "1e2", "", "NaN", 1, nil} {
		data := metadata(t, func(m map[string]any) {
			m["pricing"] = map[string]any{"type": "reference", "sources": []any{map[string]any{"provider": "coinmarketcap", "providerAssetId": "5426", "multiplier": value}}}
		})
		if _, err := v.Validate("testnet/beth/info.json", data); err == nil {
			t.Fatalf("accepted invalid multiplier %v", value)
		}
	}
}

func TestRepositoryPaths(t *testing.T) {
	v := newValidator(t)
	for _, logo := range []string{"logo.png", "assets/logo.png"} {
		root := fstest.MapFS{
			"testnet/beth/info.json": {Data: metadata(t, func(m map[string]any) { m["logo"] = logo })},
			"testnet/beth/" + logo:   {Data: []byte("logo")},
		}
		if _, err := v.ValidateRepository(root); err != nil {
			t.Fatal(err)
		}
	}
	for name, root := range map[string]fstest.MapFS{
		"symlink":               {"testnet/beth/info.json": {Mode: fs.ModeSymlink, Data: []byte("outside")}},
		"nested metadata":       {"testnet/beth/assets/info.json": {Data: metadata(t, nil)}},
		"unknown network":       {"unknown/beth/info.json": {Data: metadata(t, nil)}},
		"hidden asset":          {"testnet/.hidden/info.json": {Data: []byte("bad")}},
		"empty logo":            {"testnet/beth/info.json": {Data: metadata(t, nil)}, "testnet/beth/assets/logo.png": {}},
		"invalid other network": {"mainnet/broken/info.json": {Data: []byte("bad")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := v.ValidateRepository(root); err == nil {
				t.Fatal("accepted invalid repository")
			}
		})
	}
}
