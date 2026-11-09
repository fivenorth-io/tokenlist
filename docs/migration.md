# Existing Asset Migration

Draft inventory, 2026-11-05. Existing Wallet and Data configuration remains in place until integration testing is complete.

| Network | Registry tokens | Metadata prepared | Pending |
| --- | ---: | ---: | ---: |
| mainnet | 62 | 62 | 0 |
| devnet | 17 | 13 | 4 |
| testnet | 4 | 3 | 1 |
| Total | 83 | 78 | 5 |

## Review Before Merge


- Decimals use the approved existing Wallet convention of 10. Except for cSOL and BTC0, this is a migration assumption, not independently confirmed issuer precision. Verify before merge because decimals become immutable.
- Admin parties, instrument IDs, registry URLs and default Wallet visibility are copied from the existing registry. Legacy cSOL uses `cSOL_LEGACY`; its original instrument ID remains `cSOL`. Legacy assets stay supported but hidden by default.
- Pricing follows Wallet's current mappings and Data's CMC/CoinGecko source order. Existing fixed USD choices remain standalone, never fallbacks. Missing prices remain unavailable; this does not add new providers.
- USTY is an intentional exception: deprecated by Ember, hidden by default and unpriced in the new metadata. Wallet support stays enabled for existing holdings. The legacy Wallet configuration still shows it by default and hardcodes $1 until the shared metadata/pricing integration takes effect; these metadata files alone do not change production behavior.
- `data.listed` is true only for an existing exact asset-key/admin match. Trakx's aggregate page and Tradecraft's aggregate LP page must remain on Data's existing configuration until their group-to-instrument migration is agreed. Adding the 31 Trakx entries does not replace `/tokens/Trakx`.
- Issuer display/grouping names match between tokenlist and Wallet's legacy registry: `onRails`, `Ember`, `Excellar` (current/legacy USDXLR), `BitSafe`, `Brale`, `ACME`, `Tori Finance`, `Tradecraft`, `Trakx`, `Canton Strategic Holdings`, `Memora`, `Loop`, `Edel`, `M1X Global`, `HandlPay`, `Nuxaris`, `T-RIZE`, and `Digital Asset`. These are product grouping labels, not legal-issuer certifications. USDM1 uses M1X Global, its coordinating agent; the legal issuer is the Republic of the Marshall Islands. Wallet's CUSD/USDA/USDM1 per-token issuer overrides are removed so registry labels reach the UI. USDCx retains its existing label. Digital Asset explicitly states it is not the issuer: https://docs.digitalasset.com/usdc/xreserve/support.html. FRXUSD.B uses `Frax`. Testnet OSCoin is confirmed as Tradecraft; its metadata remains pending for a logo, and its legacy backend label has not changed yet.
- Existing logos are reused or converted to square PNGs without removing old URLs. External logos come from the URLs already configured in Wallet.
- Test the new metadata and asset-key pricing together on devnet/testnet before promoting mainnet entries to `release`. Do not remove legacy configuration yet.

## Pending Entries

No placeholder websites or logos are published. These tokens remain in the legacy registry until the missing information is supplied.

| Network | Tokens | Needed |
| --- | --- | --- |
| devnet | Tradecraft-issued CBTC, OBUSD, OSCoin, SBC | Website identified as https://tradecraft.fi/. Per Tiffany, keep pending for token-specific logos; do not substitute the Tradecraft logo. These are separate from BitSafe CBTC and Brale SBC. |
| testnet | OSCoin | Joey confirmed Tradecraft is the issuer and this is a test coin with no value. Use issuerName `Tradecraft`, website https://tradecraft.fi/ and pricing type `none`. Only the token logo remains pending. |

The devnet Digital Asset test registrar (`auth0_007c65f857f1c3d599cb6df73775`) has no instruments configured. It cannot become an asset entry until its instrument IDs are supplied. Native Canton Coin remains on its separate native-asset path, not the CIP-56 registry.

## Website Verification

Checked 2026-11-05 against Wallet's full admin party and instrument ID. A matching symbol or namespace alone does not establish the full token identity.

| Tokens | Website / evidence | Result |
| --- | --- | --- |
| devnet SOLx | [Memora verified deployments](https://www.memora.systems/docs/bridge/verified-deployments) publishes `SOLx` and the exact `memora-dp-devnet` admin in Wallet. | Added metadata and website; reused existing PNG and SOL pricing mapping. Devnet prices are test display values, not redeemable market value. |
| devnet LOOP | Existing `devnet/loop/metadata.yaml` matches Wallet's full admin and instrument. [Loop support](https://support.looptech.io/en/articles/16744297-do-i-need-an-invitation-code-to-open-a-loop-wallet) identifies wallet.looptech.io as the official wallet. | Added metadata using the current wallet website and existing logo; pricing remains unavailable. |
| devnet testUSDCx | [Digital Asset documentation](https://docs.digitalasset.com/) provides a working 192px PNG favicon. | Replaced the broken documentation favicon with its current equivalent, preserving the existing DA-brand logo choice and Wallet identity. Pricing remains unavailable. |
| Tradecraft devnet test assets | [Tradecraft](https://tradecraft.fi/) and its [documentation](https://docs.tradecraft.fi/) establish the project website. | Existing Wallet registry supplies the Tradecraft association; public sources did not establish token-specific logos. Keep pending. |
| testnet OSCoin | Joey from Tradecraft explicitly confirmed issuer ownership and no monetary value in the conversation supplied by Tiffany. He supplied the [testnet pool token endpoint](https://tradecraft.validator.test.canton.obsidian.systems/amm-http-api/tokenB/CC/OSCoin). | Issuer `Tradecraft`, website https://tradecraft.fi/, pricing `none`. Keep pending only for its token-specific logo. Preserve the existing testnet admin; the pool trading party is not the token admin. Do not carry the legacy OSCoin fixed-$1 assumption into shared metadata. |
| eXAU, eXAG | [Ember](https://ember.so/) public application configuration references both tokens. | Added website links using existing Wallet identities and reference pricing (CMC gold 39344 / silver 39343). Converted existing SVG logos to PNG, retaining originals. The website association does not independently verify the full admin parties. |
| USTY | Tiffany relayed Ember's confirmation that USTY support was deprecated in favor of commodities. [Ember website](https://ember.so/). | Added with Wallet enabled, default visibility off, Data listing off and pricing `none`. Existing holdings remain supported. Transfer/redemption availability still needs issuer confirmation. |
| FRXUSD.B | [Send integration guide](https://info.send.it/docs/asset-relayer/integration-guide) publishes our exact mainnet admin and `FRXUSD.B`. | Added metadata in `mainnet/frax/info.json`. Existing logo converted to compliant PNG; original retained. |
| USDM1 | [USDM1](https://usdm1.com/) identifies the instrument, Republic of the Marshall Islands as issuer, and M1X Global as coordinating agent. | Website linked for migration using Wallet's existing token identity. Exact Canton admin/instrument pair not independently confirmed by the website; review before merge. |
| aCBTC, aCC, aUSDCx | [ACME](https://acmemarkets.cc/) is linked by the [Canton ecosystem directory](https://www.cantonecosystem.com/ecosystem/acme). Wallet labels their shared registrar ACME/Hello Moon. | Added metadata using existing Wallet identities, pricing mappings and logos padded to square PNGs. Website association is not independent verification of the full token identities. |
| TC CBTC/CC LP, TC CC/USDCx LP | [Tradecraft](https://tradecraft.fi/) website; existing Wallet registry provides identities and Tradecraft logos. | Added metadata, preserving hidden-by-default visibility and unavailable pricing. Data listing remains off pending Jieun's decision on separate token pages versus the existing aggregate page. Keep the existing Data page configuration. |
| HANDL | [HANDL token page](https://www.handlpay.com/token) links its [bridge](https://bridge.handlpay.com/). The public bridge configuration uses `bridge::12207ac147ad3e95894e7a507da3f04a819e8996f0bdf31c084ab3b77e51746b03cf`. | Website linked for migration using Wallet's existing token identity and HandlPay display name. Bridge namespace matches, but it is a different party from `handl-issuer`; this is not independent verification of the full token identity. |
| BTC0 | [BTC0 token identity](https://btc0.network/integration/token-identity) publishes the matching mainnet/devnet admin parties, BTC0 symbol and 10 decimals. Verified directly from the page's network table configuration. | Added mainnet and devnet metadata using existing logos and pricing mappings. |

FRXUSD.B retains the approved Wallet decimal convention of 10. Send's guide describes ERC-20 decimals and six-decimal bridge transfer precision; those are not independent confirmation of the Canton metadata decimal field. Pricing stays unavailable rather than assuming a $1 peg or native frxUSD price.

## Sources

- Identities and visibility: Wallet's `pkg/registry/registry.go`.
- Data listing keys: Lighthouse's `app/config/config.go`.
- Price mappings: Wallet's `src/constants/tokens.constant.ts` and `src/contexts/walletBalance.context.tsx`; Lighthouse's `app/pkg/pricing/sources.go`.
- [cSOL identity and decimals](https://www.csol.network/integration/token-identity).
- [BitSafe assets](https://bitsafe.finance), [Trakx Canton integration](https://trakx.io/resources/news/crypto-indices-on-chain-on-canton/), [Brale](https://brale.xyz), [Tori](https://tori.finance).
- [Nuxaris cBNB documentation](https://www.nuxaris.com/docs), [EDELx](https://docs.edel.finance/token/overview), [Hecto](https://app.hecto.inc), [CNTN](https://www.cantonstrategic.com), [Excellar](https://www.excellar.finance), [cETH](https://www.ceth.network).
- [USDCx on Canton](https://www.circle.com/blog/usdcx-on-canton-now-available-via-circle-xreserve), [Digital Asset documentation](https://docs.digitalasset.com).
- [T-RIZE's official organization and website](https://github.com/T-RIZE-Group).
