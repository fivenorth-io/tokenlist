package validator

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Keys reserved for Loop Data markets and aliases.
var reservedKeys = strings.Fields("amulet btc eth xau xag sol bnb hype rize usdm1 trusd strusd cc cbtc ceth exau exag csol cbnb chype")

// Approved existing identities allowed to use reserved keys.
// Update only through review.
//
//go:embed reserved-assets.json
var reservedAssetsJSON []byte

type reservedAsset struct {
	Network      string `json:"network"`
	AssetKey     string `json:"assetKey"`
	AdminParty   string `json:"adminParty"`
	InstrumentID string `json:"instrumentId"`
}

func IsReservedKey(key string) bool {
	for _, reserved := range reservedKeys {
		if strings.ToLower(key) == reserved {
			return true
		}
	}
	return false
}

func validateReservedKey(entry Entry) error {
	if !IsReservedKey(entry.AssetKey) {
		return nil
	}
	var existing []reservedAsset
	if err := json.Unmarshal(reservedAssetsJSON, &existing); err != nil {
		return fmt.Errorf("invalid reserved asset list: %w", err)
	}
	for _, asset := range existing {
		if asset.Network == entry.Network && asset.AssetKey == entry.AssetKey && asset.AdminParty == entry.AdminParty && asset.InstrumentID == entry.InstrumentID {
			return nil
		}
	}
	return fmt.Errorf("%s: assetKey %q is reserved", entry.Path, entry.AssetKey)
}

// Requires schema-validated source fields.
func validateProviderOrder(fields map[string]any) error {
	pricing := fields["pricing"].(map[string]any)
	sources, _ := pricing["sources"].([]any)
	seenCoinGecko := false
	seenProviders := make(map[string]bool)
	for _, raw := range sources {
		source := raw.(map[string]any)
		provider := source["provider"].(string)
		if seenProviders[provider] {
			return fmt.Errorf("provider %q can appear only once in a source list", provider)
		}
		seenProviders[provider] = true
		// Issuer prices are never market prices, so the CMC and CoinGecko order doesn't apply to them.
		if provider == "issuer" {
			continue
		}
		if provider == "coingecko" {
			seenCoinGecko = true
			continue
		}
		if _, err := strconv.ParseInt(source["providerAssetId"].(string), 10, 64); err != nil {
			return fmt.Errorf("CoinMarketCap providerAssetId must fit a positive int64")
		}
		if seenCoinGecko {
			return fmt.Errorf("CoinMarketCap sources must precede CoinGecko sources")
		}
	}
	return nil
}
