package validator

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// secp256k1 keys in lowercase hex DER (uncompressed SPKI): the generator and two Canton Data Standard
// v0.2.0 test vectors (tests-crypto/daml/Test/ExternalSignerTest.daml).
const (
	keyG    = "3056301006072a8648ce3d020106052b8104000a0342000479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
	keyCDS1 = "3056301006072a8648ce3d020106052b8104000a034200041c05f66cb995dcfb3742bc22dcd7f48d2c8cdbb1314fc8a6eec55120708ac667651e2db08cc4924d42e99cdf9d1544c86b30b13a31e1c824dce2c7af9f58e12d"
	keyCDS2 = "3056301006072a8648ce3d020106052b8104000a034200043432ed27a36d76b5badc2f5171413ba858dc7a53a78df3caeb439f1e81b11210b5f55e741b31fae62c9aeb1300e514c42a2f5fcd995994ba82b2547150417ae7"
)

func issuerKey(kid, publicKey string) map[string]any {
	return map[string]any{"kid": kid, "publicKey": publicKey}
}

func issuerSource(feed, kind string) map[string]any {
	return map[string]any{"provider": "issuer", "providerAssetId": feed, "kind": kind}
}

func TestIssuerKeys(t *testing.T) {
	v := newValidator(t)
	prefix, gx, gy := keyG[:48], keyG[48:112], keyG[112:]
	for _, tc := range []struct {
		name  string
		keys  []any
		valid bool
	}{
		{"generator", []any{issuerKey("2026-11", keyG)}, true},
		{"data standard key 1", []any{issuerKey("2026-11", keyCDS1)}, true},
		{"data standard key 2", []any{issuerKey("2026-11", keyCDS2)}, true},
		{"two keys", []any{issuerKey("2026-05", keyG), issuerKey("2026-11", keyCDS1)}, true},
		{"kid punctuation", []any{issuerKey("Key_1.v-2", keyG)}, true},
		{"kid of 64 characters", []any{issuerKey(strings.Repeat("k", 64), keyG)}, true},
		{"uppercase", []any{issuerKey("k", strings.ToUpper(keyG))}, false},
		{"compressed", []any{issuerKey("k", "3036301006072a8648ce3d020106052b8104000a03220002"+gx)}, false},
		{"bare point", []any{issuerKey("k", "04"+gx+gy)}, false},
		{"private key", []any{issuerKey("k", strings.Repeat("3f", 32))}, false},
		{"P-256", []any{issuerKey("k", "3059301306072a8648ce3d020106082a8648ce3d030107034200"+"04"+gx+gy)}, false},
		{"off curve", []any{issuerKey("k", keyG[:len(keyG)-1]+"9")}, false},
		// Both points satisfy the curve equation mod p, so only the x, y < p check rejects them.
		{"x not below p", []any{issuerKey("k", prefix+"fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc30"+"4218f20ae6c646b363db68605822fb14264ca8d2587fdd6fbc750d587e76a7ee")}, false},
		{"y not below p", []any{issuerKey("k", prefix+"1fe1e5ef3fceb5c135ab7741333ce5a6e80d68167653f6b2b24bcbcfaaaff507"+"fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc30")}, false},
		{"zero point", []any{issuerKey("k", prefix+strings.Repeat("0", 128))}, false},
		{"duplicate kid", []any{issuerKey("k", keyG), issuerKey("k", keyCDS1)}, false},
		{"kids that differ only in case", []any{issuerKey("k", keyG), issuerKey("K", keyCDS1)}, false},
		{"duplicate publicKey", []any{issuerKey("a", keyG), issuerKey("b", keyG)}, false},
		{"three keys", []any{issuerKey("a", keyG), issuerKey("b", keyCDS1), issuerKey("c", keyCDS2)}, false},
		{"no keys", []any{}, false},
		{"empty kid", []any{issuerKey("", keyG)}, false},
		{"kid with a space", []any{issuerKey("2026 11", keyG)}, false},
		{"kid of 65 characters", []any{issuerKey(strings.Repeat("k", 65), keyG)}, false},
		{"extra field", []any{map[string]any{"kid": "k", "publicKey": keyG, "validFrom": "2026-11-20T00:00:00Z"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The fixture's pricing is none: keys don't need an issuer source.
			data := metadata(t, func(m map[string]any) { m["issuerKeys"] = tc.keys })
			if _, err := v.Validate("testnet/beth/info.json", data); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestIssuerSources(t *testing.T) {
	v := newValidator(t)
	cmc := map[string]any{"provider": "coinmarketcap", "providerAssetId": "5426"}
	gecko := map[string]any{"provider": "coingecko", "providerAssetId": "solana"}
	nav := issuerSource("testnet:beth-nav", "nav")
	navWith := func(field string, value any) map[string]any {
		source := issuerSource("testnet:beth-nav", "nav")
		source[field] = value
		return source
	}
	for _, tc := range []struct {
		name    string
		network string
		sources []any
		valid   bool
	}{
		{"mainnet feed", "mainnet", []any{issuerSource("mainnet:usty-nav", "nav")}, true},
		{"pair feed", "testnet", []any{issuerSource("testnet:BTC/USD", "issuer_reference")}, true},
		{"rate feed", "devnet", []any{issuerSource("devnet:KK_RFR_EURCHF", "nav")}, true},
		{"issuer reference", "mainnet", []any{issuerSource("mainnet:usty-nav", "issuer_reference")}, true},
		{"before market sources", "testnet", []any{nav, cmc, gecko}, true},
		{"after market sources", "testnet", []any{cmc, gecko, nav}, true},
		{"between market sources", "testnet", []any{cmc, nav, gecko}, true},
		{"maxAge", "testnet", []any{navWith("maxAge", 86400)}, true},
		{"128 characters", "testnet", []any{issuerSource("testnet:"+strings.Repeat("a", 120), "nav")}, true},
		{"129 characters", "testnet", []any{issuerSource("testnet:"+strings.Repeat("a", 121), "nav")}, false},
		{"second colon", "testnet", []any{issuerSource("testnet:x:y", "nav")}, false},
		{"multiplier 1", "testnet", []any{navWith("multiplier", "1")}, false},
		{"multiplier 1000", "testnet", []any{navWith("multiplier", "1000")}, false},
		{"market order still applies", "testnet", []any{gecko, nav, cmc}, false},
		{"no kind", "testnet", []any{map[string]any{"provider": "issuer", "providerAssetId": "testnet:beth-nav"}}, false},
		{"market kind", "testnet", []any{issuerSource("testnet:beth-nav", "market")}, false},
		{"other network", "testnet", []any{issuerSource("mainnet:beth-nav", "nav")}, false},
		{"unknown network", "testnet", []any{issuerSource("foo:x", "nav")}, false},
		{"no network", "testnet", []any{issuerSource("beth-nav", "nav")}, false},
		{"empty ID", "mainnet", []any{issuerSource("mainnet:", "nav")}, false},
		{"capitalized network", "mainnet", []any{issuerSource("Mainnet:x", "nav")}, false},
		{"space in ID", "testnet", []any{issuerSource("testnet:BTC USD", "nav")}, false},
		{"two issuer sources", "testnet", []any{nav, issuerSource("testnet:beth-ref", "issuer_reference")}, false},
	} {
		// An issuer source needs direct pricing, so every case fails under reference pricing.
		for _, pricing := range []string{"direct", "reference"} {
			t.Run(tc.name+"/"+pricing, func(t *testing.T) {
				data := metadata(t, func(m map[string]any) {
					m["network"], m["issuerKeys"] = tc.network, []any{issuerKey("2026-11", keyG)}
					m["pricing"] = map[string]any{"type": pricing, "sources": tc.sources}
				})
				valid := tc.valid && pricing == "direct"
				if _, err := v.Validate(tc.network+"/beth/info.json", data); (err == nil) != valid {
					t.Fatalf("valid=%v: %v", valid, err)
				}
			})
		}
	}
	data := metadata(t, func(m map[string]any) { m["pricing"] = map[string]any{"type": "direct", "sources": []any{nav}} })
	if _, err := v.Validate("testnet/beth/info.json", data); err == nil {
		t.Fatal("accepted an issuer source without issuerKeys")
	}
}

func TestIssuerKeysAndFeedsAcrossEntries(t *testing.T) {
	v := newValidator(t)
	card := func(network, asset, publicKey, feed string) []byte {
		return metadata(t, func(m map[string]any) {
			m["network"], m["assetKey"], m["instrumentId"] = network, asset, asset
			m["issuerKeys"] = []any{issuerKey("2026-11", publicKey)}
			if feed != "" {
				m["pricing"] = map[string]any{"type": "direct", "sources": []any{issuerSource(feed, "nav")}}
			}
		})
	}
	entry := func(network, asset, publicKey, feed string) Entry {
		t.Helper()
		e, err := v.Validate(network+"/"+strings.ToLower(asset)+"/info.json", card(network, asset, publicKey, feed))
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	beth := entry("testnet", "BETH", keyG, "testnet:BTC/USD")
	if err := ValidateSet([]Entry{beth, entry("testnet", "USTY", keyG, "testnet:usty-nav")}); err != nil {
		t.Fatal("one key may sign for several entries in a network", err)
	}
	if err := ValidateSet([]Entry{beth, entry("mainnet", "BETH", keyCDS1, "mainnet:BTC/USD")}); err != nil {
		t.Fatal("a separate key per network", err)
	}
	if ValidateSet([]Entry{beth, entry("mainnet", "BETH", keyG, "")}) == nil {
		t.Fatal("accepted one key in two networks")
	}
	for _, feed := range []string{"testnet:BTC/USD", "testnet:btc/usd"} {
		if ValidateSet([]Entry{beth, entry("testnet", "USTY", keyG, feed)}) == nil {
			t.Fatalf("accepted feed ID %s on a second entry", feed)
		}
	}
	root := fstest.MapFS{
		"testnet/beth/info.json":       {Data: card("testnet", "BETH", keyG, "")},
		"testnet/beth/assets/logo.png": {Data: testPNG(t, 256, 256)},
		"mainnet/beth/info.json":       {Data: card("mainnet", "BETH", keyCDS1, "")},
		"mainnet/beth/assets/logo.png": {Data: testPNG(t, 256, 256)},
	}
	if _, err := v.ValidateRepository(root); err != nil {
		t.Fatal(err)
	}
	root["mainnet/beth/info.json"] = &fstest.MapFile{Data: card("mainnet", "BETH", keyG, "")}
	if _, err := v.ValidateRepository(root); err == nil {
		t.Fatal("accepted one key in two networks across the repository")
	}
}

func TestIssuerKeysPerAdminParty(t *testing.T) {
	v := newValidator(t)
	issuer, other := "issuer::1220"+strings.Repeat("a", 64), "other::1220"+strings.Repeat("b", 64)
	type card struct {
		network, admin string
		keys           []any
	}
	data := func(c card, asset string) []byte {
		return metadata(t, func(m map[string]any) {
			m["network"], m["adminParty"], m["assetKey"], m["instrumentId"], m["issuerKeys"] = c.network, c.admin, asset, asset, c.keys
			if c.admin == other {
				m["issuerName"], m["registryUrl"] = "Other", "https://other.example.com/registry"
			}
		})
	}
	entries := func(cards []card) []Entry {
		t.Helper()
		var list []Entry
		for i, c := range cards {
			asset := string(rune('A' + i))
			e, err := v.Validate(c.network+"/"+strings.ToLower(asset)+"/info.json", data(c, asset))
			if err != nil {
				t.Fatal(err)
			}
			list = append(list, e)
		}
		return list
	}
	for _, tc := range []struct {
		name  string
		cards []card
		err   string // a part of the error, or "" when the entries are valid
	}{
		{"one key and kid on three entries", []card{
			{"testnet", issuer, []any{issuerKey("2026-11", keyG)}},
			{"testnet", issuer, []any{issuerKey("2026-11", keyG)}},
			{"testnet", issuer, []any{issuerKey("2026-11", keyG)}},
		}, ""},
		{"rotation", []card{
			{"testnet", issuer, []any{issuerKey("2026-05", keyG), issuerKey("2026-11", keyCDS1)}},
			{"testnet", issuer, []any{issuerKey("2026-05", keyG)}},
			{"testnet", issuer, []any{issuerKey("2026-11", keyCDS1)}},
		}, ""},
		{"two keys in either order", []card{
			{"testnet", issuer, []any{issuerKey("2026-05", keyG), issuerKey("2026-11", keyCDS1)}},
			{"testnet", issuer, []any{issuerKey("2026-11", keyCDS1), issuerKey("2026-05", keyG)}},
		}, ""},
		{"one kid in two admin parties", []card{
			{"testnet", issuer, []any{issuerKey("2026-11", keyG)}},
			{"testnet", other, []any{issuerKey("2026-11", keyCDS1)}},
		}, ""},
		{"one kid in two networks", []card{
			{"testnet", issuer, []any{issuerKey("2026-11", keyG)}},
			{"mainnet", issuer, []any{issuerKey("2026-11", keyCDS1)}},
		}, ""},
		{"kid names two keys", []card{
			{"testnet", issuer, []any{issuerKey("k", keyG)}},
			{"testnet", issuer, []any{issuerKey("k", keyCDS1)}},
		}, "already names another key"},
		{"kids that differ only in case name two keys", []card{
			{"testnet", issuer, []any{issuerKey("k", keyG)}},
			{"testnet", issuer, []any{issuerKey("K", keyCDS1)}},
		}, "already names another key"},
		{"second kid names another key", []card{
			{"testnet", issuer, []any{issuerKey("a", keyG), issuerKey("b", keyCDS1)}},
			{"testnet", issuer, []any{issuerKey("b", keyCDS2)}},
		}, "already names another key"},
		{"key under two kids", []card{
			{"testnet", issuer, []any{issuerKey("a", keyG)}},
			{"testnet", issuer, []any{issuerKey("b", keyG)}},
		}, "is already listed as"},
		{"key under kids that differ only in case", []card{
			{"testnet", issuer, []any{issuerKey("k", keyG)}},
			{"testnet", issuer, []any{issuerKey("K", keyG)}},
		}, "is already listed as"},
		{"key of two admin parties", []card{
			{"testnet", issuer, []any{issuerKey("k", keyG)}},
			{"testnet", other, []any{issuerKey("k", keyG)}},
		}, "already used by another admin party"},
		{"key of two admin parties under two kids", []card{
			{"testnet", issuer, []any{issuerKey("a", keyG)}},
			{"testnet", other, []any{issuerKey("b", keyG)}},
		}, "already used by another admin party"},
		{"key in two networks of two admin parties", []card{
			{"testnet", issuer, []any{issuerKey("k", keyG)}},
			{"mainnet", other, []any{issuerKey("k", keyG)}},
		}, "use a separate key per network"},
	} {
		for _, order := range []string{"forward", "reverse"} {
			set := entries(tc.cards)
			if order == "reverse" {
				slices.Reverse(set)
			}
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				err := ValidateSet(set)
				if tc.err == "" && err != nil {
					t.Fatal(err)
				}
				if tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
					t.Fatalf("want an error with %q, got %v", tc.err, err)
				}
			})
		}
	}
	a, b := card{"testnet", issuer, []any{issuerKey("2026-11", keyG)}}, card{"testnet", other, []any{issuerKey("2026-11", keyCDS1)}}
	root := fstest.MapFS{
		"testnet/a/info.json":       {Data: data(a, "A")},
		"testnet/a/assets/logo.png": {Data: testPNG(t, 256, 256)},
		"testnet/b/info.json":       {Data: data(b, "B")},
		"testnet/b/assets/logo.png": {Data: testPNG(t, 256, 256)},
	}
	if _, err := v.ValidateRepository(root); err != nil {
		t.Fatal(err)
	}
	b.keys = []any{issuerKey("2026-11", keyG)}
	root["testnet/b/info.json"] = &fstest.MapFile{Data: data(b, "B")}
	if _, err := v.ValidateRepository(root); err == nil || !strings.Contains(err.Error(), "already used by another admin party") {
		t.Fatalf("one key for two admin parties across the repository: %v", err)
	}
}
