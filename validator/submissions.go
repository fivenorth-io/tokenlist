package validator

import (
	"fmt"
	"strconv"
)

// Called after schema validation, which verifies source fields and provider names.
func validateProviderOrder(fields map[string]any) error {
	pricing := fields["pricing"].(map[string]any)
	sources, _ := pricing["sources"].([]any)
	seenCoinGecko := false
	for _, raw := range sources {
		source := raw.(map[string]any)
		if source["provider"] == "coingecko" {
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
