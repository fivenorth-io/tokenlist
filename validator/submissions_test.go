package validator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProviderSubmissions(t *testing.T) {
	v := newValidator(t)
	for _, tc := range []struct {
		name    string
		sources []map[string]any
		valid   bool
	}{
		{"cmc", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1"}}, true},
		{"gecko only", []map[string]any{{"provider": "coingecko", "providerAssetId": "tori-staked-trusd"}}, true},
		{"ordered", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1"}, {"provider": "coingecko", "providerAssetId": "bitcoin"}}, true},
		{"reversed", []map[string]any{{"provider": "coingecko", "providerAssetId": "bitcoin"}, {"provider": "coinmarketcap", "providerAssetId": "1"}}, false},
		{"repeated cmc", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1"}, {"provider": "coinmarketcap", "providerAssetId": "1027"}}, false},
		{"repeated gecko", []map[string]any{{"provider": "coingecko", "providerAssetId": "bitcoin"}, {"provider": "coingecko", "providerAssetId": "ethereum"}}, false},
		{"identical source", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1"}, {"provider": "coinmarketcap", "providerAssetId": "1"}}, false},
		{"unsupported", []map[string]any{{"provider": "pyth", "providerAssetId": "feed"}}, false},
		{"cmc text", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "bitcoin"}}, false},
		{"cmc zero", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "0"}}, false},
		{"cmc leading zero", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "01"}}, false},
		{"cmc overflow", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "9223372036854775808"}}, false},
		{"cmc fraction", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1.5"}}, false},
		{"cmc multiple", []map[string]any{{"provider": "coinmarketcap", "providerAssetId": "1,2"}}, false},
		{"gecko multiple", []map[string]any{{"provider": "coingecko", "providerAssetId": "bitcoin,ethereum"}}, false},
		{"gecko url", []map[string]any{{"provider": "coingecko", "providerAssetId": "https://coingecko.com/bitcoin"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []string{"direct", "reference"} {
				data := metadata(t, func(m map[string]any) { m["pricing"] = map[string]any{"type": kind, "sources": tc.sources} })
				_, err := v.Validate("testnet/beth/info.json", data)
				if (err == nil) != tc.valid {
					t.Fatalf("%s valid=%v: %v", kind, tc.valid, err)
				}
			}
		})
	}
}

func TestSubmissionTextLimits(t *testing.T) {
	v := newValidator(t)
	for field, limit := range map[string]int{"assetKey": 64, "name": 128, "symbol": 64, "issuerName": 128, "instrumentId": 128} {
		for _, extra := range []int{0, 1} {
			data := metadata(t, func(m map[string]any) { m[field] = strings.Repeat("a", limit+extra) })
			_, err := v.Validate("testnet/beth/info.json", data)
			if (err == nil) != (extra == 0) {
				t.Errorf("%s length %d: %v", field, limit+extra, err)
			}
		}
	}
	for _, key := range []string{"A,B", "A/B", "A\nB", "A\tB", "A%B", "A&B", "A?B", "A#B", "BÉTH"} {
		if _, err := v.Validate("testnet/beth/info.json", metadata(t, func(m map[string]any) { m["assetKey"] = key })); err == nil {
			t.Errorf("accepted unsafe key %q", key)
		}
	}
	for _, key := range []string{"FRXUSD.B", "TC CC-USDCx LP", "cSOL_LEGACY"} {
		if _, err := v.Validate("testnet/beth/info.json", metadata(t, func(m map[string]any) { m["assetKey"] = key })); err != nil {
			t.Errorf("rejected supported key %q: %v", key, err)
		}
	}
	for _, field := range []string{"name", "issuerName", "symbol", "instrumentId"} {
		if _, err := v.Validate("testnet/beth/info.json", metadata(t, func(m map[string]any) { m[field] = "a\nb" })); err == nil {
			t.Errorf("accepted control character in %s", field)
		}
	}
}

func TestReservedKeys(t *testing.T) {
	v := newValidator(t)
	for _, key := range reservedKeys {
		for _, casing := range []string{key, strings.ToUpper(key)} {
			if _, err := v.Validate("testnet/beth/info.json", metadata(t, func(m map[string]any) { m["assetKey"] = casing })); err == nil {
				t.Errorf("accepted reserved key %s", casing)
			}
		}
	}
	var existing []reservedAsset
	if err := json.Unmarshal(reservedAssetsJSON, &existing); err != nil {
		t.Fatal(err)
	}
	if len(existing) == 0 {
		t.Fatal("missing compatibility entries")
	}
	for _, asset := range existing {
		entry := Entry{Network: asset.Network, AssetKey: asset.AssetKey, AdminParty: asset.AdminParty, InstrumentID: asset.InstrumentID}
		if err := validateReservedKey(entry); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"network", "admin", "instrument", "case"} {
			changed := entry
			switch field {
			case "network":
				changed.Network = "unknown"
			case "admin":
				changed.AdminParty = "other::1220" + strings.Repeat("b", 64)
			case "instrument":
				changed.InstrumentID = "other"
			case "case":
				changed.AssetKey = strings.ToUpper(entry.AssetKey)
				if changed.AssetKey == entry.AssetKey {
					changed.AssetKey = strings.ToLower(entry.AssetKey)
				}
			}
			if err := validateReservedKey(changed); err == nil {
				t.Errorf("allowed %s takeover of %s", field, entry.AssetKey)
			}
		}
	}
}

func TestExistingAssetsMeetSubmissionRules(t *testing.T) {
	entries, err := newValidator(t).ValidateRepository(os.DirFS(".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no existing assets validated")
	}
}

func TestAdminPartyLength(t *testing.T) {
	v := newValidator(t)
	for _, length := range []int{255, 256} {
		data := metadata(t, func(m map[string]any) {
			suffix := "::1220" + strings.Repeat("a", 64)
			m["adminParty"] = strings.Repeat("a", length-len(suffix)) + suffix
		})
		_, err := v.Validate("testnet/beth/info.json", data)
		if (err == nil) != (length == 255) {
			t.Errorf("adminParty length %d: %v", length, err)
		}
	}
}
