package validator

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
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
		"SVG logo":            func(m map[string]any) { m["logo"] = "logo.svg" },
		"JPEG logo":           func(m map[string]any) { m["logo"] = "logo.jpeg" },
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
		"testnet/beth/assets/logo.png":   {Data: testPNG(t, 256, 256)},
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

func TestReadIdentitiesIgnoresNewMetadataRules(t *testing.T) {
	v := newValidator(t)
	old := metadata(t, func(m map[string]any) {
		m["oldField"] = true
		m["website"] = "http://old.example.com"
		m["pricing"] = "old pricing format"
		m["logo"] = "missing.svg"
	})
	root := fstest.MapFS{"testnet/beth/info.json": {Data: old}}
	if _, err := v.ValidateRepository(root); err == nil {
		t.Fatal("fixture should fail current validation")
	}
	previous, err := ReadIdentities(root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := v.Validate("testnet/beth/info.json", metadata(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(previous, []Entry{current}); err != nil {
		t.Fatal(err)
	}
	if ValidateTransition(previous, nil) == nil {
		t.Fatal("accepted removal")
	}
	renamed := current
	renamed.AssetKey = "renamed"
	if ValidateTransition(previous, []Entry{renamed}) == nil {
		t.Fatal("accepted key rename")
	}
	changed := current
	changed.AdminParty = "different"
	if ValidateTransition(previous, []Entry{changed}) == nil {
		t.Fatal("accepted identity change")
	}
}

func TestReadIdentitiesRejectsUnreadableIdentity(t *testing.T) {
	for name, body := range map[string][]byte{
		"malformed JSON": []byte(`{`),
		"missing field":  metadata(t, func(m map[string]any) { delete(m, "assetKey") }),
		"null field":     metadata(t, func(m map[string]any) { m["adminParty"] = nil }),
		"wrong type":     metadata(t, func(m map[string]any) { m["instrumentId"] = 1 }),
		"wrong folder":   metadata(t, func(m map[string]any) { m["network"] = "mainnet" }),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadIdentities(fstest.MapFS{"testnet/beth/info.json": {Data: body}}); err == nil {
				t.Fatal("accepted unreadable base identity")
			}
		})
	}
	if entries, err := ReadIdentities(fstest.MapFS{"testnet/beth/logo.png": {Data: []byte("logo")}}); err != nil || len(entries) != 0 {
		t.Fatalf("logo-only base: %v, %v", entries, err)
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

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestPNGContent(t *testing.T) {
	v := newValidator(t)
	valid := testPNG(t, 256, 256)
	for _, tc := range []struct {
		name  string
		body  []byte
		valid bool
	}{
		{"minimum", testPNG(t, 128, 128), true},
		{"maximum", testPNG(t, 1024, 1024), true},
		{"small", testPNG(t, 127, 127), false},
		{"large", testPNG(t, 1025, 1025), false},
		{"not square", testPNG(t, 256, 128), false},
		{"renamed SVG", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`), false},
		{"truncated image", valid[:len(valid)-15], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.ValidateRepository(fstest.MapFS{
				"testnet/beth/info.json":       {Data: metadata(t, nil)},
				"testnet/beth/assets/logo.png": {Data: tc.body},
			})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
	if _, err := v.ValidateRepository(fstest.MapFS{"mainnet/legacy/logo.svg": {Data: []byte(`<svg/>`)}}); err != nil {
		t.Fatal("unreferenced legacy SVG must remain allowed", err)
	}
}

func TestLogoSize(t *testing.T) {
	v := newValidator(t)
	for _, size := range []int{maxLogoBytes, maxLogoBytes + 1} {
		for _, referenced := range []bool{false, true} {
			root := fstest.MapFS{"testnet/beth/assets/logo.png": {Data: make([]byte, size)}}
			copy(root["testnet/beth/assets/logo.png"].Data, testPNG(t, 128, 128))
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

func TestDecimalsAreStable(t *testing.T) {
	v := newValidator(t)
	for _, precision := range []int{0, 10} {
		data := metadata(t, func(m map[string]any) { m["decimals"] = precision })
		previous, err := ReadIdentities(fstest.MapFS{"testnet/beth/info.json": {Data: data}})
		if err != nil {
			t.Fatal(err)
		}
		current, err := v.Validate("testnet/beth/info.json", data)
		if err != nil {
			t.Fatal(err)
		}
		if current.Decimals != int64(precision) || previous[0].Decimals != int64(precision) {
			t.Fatal("decimals were not preserved")
		}
		if err := ValidateTransition(previous, []Entry{current}); err != nil {
			t.Fatal(err)
		}
		current.Decimals = 6
		if ValidateTransition(previous, []Entry{current}) == nil {
			t.Fatal("accepted decimals change")
		}
		if err := ValidateTransition(nil, []Entry{current}); err != nil {
			t.Fatal("new assets may choose decimals", err)
		}
	}
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) { delete(m, "decimals") },
		func(m map[string]any) { m["decimals"] = nil },
		func(m map[string]any) { m["decimals"] = "10" },
	} {
		if _, err := ReadIdentities(fstest.MapFS{"testnet/beth/info.json": {Data: metadata(t, mutate)}}); err == nil {
			t.Fatal("accepted missing or invalid base decimals")
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
			"testnet/beth/" + logo:   {Data: testPNG(t, 256, 256)},
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
