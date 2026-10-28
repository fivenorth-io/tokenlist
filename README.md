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
- Asset keys are case-sensitive and unique per network. Preserve existing keys, on-chain identities, and decimals; disable assets instead of deleting them.
- Assets with the same network and admin party must share an issuer name and registry URL.
- Direct/reference pricing uses ordered sources with a positive decimal-string multiplier (default `"1"`). Fixed USD pricing and unavailable pricing are standalone choices, never fallbacks.

CI checks metadata, logos, consistency, and changes to existing identities. Reviewers verify issuer and pricing information. Merging does not guarantee immediate availability in every app.
