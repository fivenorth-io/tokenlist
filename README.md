# Loop Tokenlist

Shared asset metadata and logos for Loop Wallet and Loop Data.

## Submit an Asset

1. Fork the repo and create a branch.
2. Add `<network>/<asset-folder>/info.json` and a logo. Networks: `mainnet`, `devnet`, `testnet`.
3. Follow [the metadata schema](schema/asset.schema.json) and run:

   ```sh
   go test ./...
   go run ./cmd/validate-assets
   ```

4. Open a PR with public issuer documentation. Address CI failures and reviewer feedback before merge.

## Rules

- Submit public information only. No secrets, personal data, private endpoints, or authenticated URLs. Validation does not detect all sensitive data.
- Logos must be PNG, square, 128-1024 pixels per side, and no larger than 2 MB. Recommended: 256x256 or 512x512. Paths are relative to `info.json`.
- Asset keys are unique per network regardless of case, so `USDCx` and `usdcx` cannot both exist in one network. Keep a key's case as written; changing it counts as a rename. Preserve existing keys, on-chain identities, and decimals; disable assets instead of deleting them.
- Assets with the same network and admin party must share an issuer name and registry URL.
- Direct/reference pricing uses ordered sources with a positive decimal-string multiplier (default `"1"`). Fixed USD pricing and unavailable pricing are standalone choices, never fallbacks.
- Supported providers are `coinmarketcap` and `coingecko`. Use a positive numeric CMC ID (no leading zeros, at most signed 64-bit) or a lowercase CoinGecko API ID, not a URL. CMC sources must come before CoinGecko when both are present; either provider can be used alone. Each provider can appear only once per source list.
- A source may set `kind`: `market` (the default), `nav`, or `issuer_reference`. Sources of different kinds never stand in for each other. A source may also set `maxAge`, the age in seconds after which a consumer asks the next source of the same kind; `0` accepts any age and the largest value is `604800` (7 days).
- Leave `kind` and `maxAge` out of entries until Loop Wallet and Loop Data run validator v0.2.0. An older validator rejects them.
- Asset keys are at most 64 characters, start with a letter or digit, and use ASCII letters, digits, spaces, `.`, `_`, or `-`. Names, issuer names, instrument IDs, and provider IDs are at most 128 characters; symbols are at most 64. Text fields cannot contain control characters. URLs are limited to 2048 characters, admin parties to 255, and logo paths to 256.
- Market keys and aliases used by Loop Data (such as `btc`, `eth`, and `cbtc`) are reserved regardless of case. Existing entries are allowed only for the identities recorded in `validator/reserved-assets.json`; new issuers must choose a different key. Display symbols do not need to match asset keys.

CI checks metadata, logos, consistency, and changes to existing identities. Reviewers verify issuer and pricing information. Merging does not guarantee immediate availability in every app.
