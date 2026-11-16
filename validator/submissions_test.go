package validator

import "testing"

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
