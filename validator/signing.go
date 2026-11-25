package validator

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// IssuerKey is a key the issuer signs an entry's prices with.
type IssuerKey struct {
	Kid       string
	PublicKey string
}

// The field prime of secp256k1.
var secp256k1P, _ = new(big.Int).SetString("fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f", 16)

// Requires schema-validated fields.
func issuerKeys(fields map[string]any) []IssuerKey {
	list, _ := fields["issuerKeys"].([]any)
	var keys []IssuerKey
	for _, raw := range list {
		key := raw.(map[string]any)
		keys = append(keys, IssuerKey{Kid: key["kid"].(string), PublicKey: key["publicKey"].(string)})
	}
	return keys
}

// Requires schema-validated fields. Returns nil when no source is an issuer source.
func findIssuerSource(fields map[string]any) map[string]any {
	sources, _ := fields["pricing"].(map[string]any)["sources"].([]any)
	for _, raw := range sources {
		if source := raw.(map[string]any); source["provider"] == "issuer" {
			return source
		}
	}
	return nil
}

// Requires schema-validated fields. Returns "" when no source is an issuer source.
func issuerFeedID(fields map[string]any) string {
	if source := findIssuerSource(fields); source != nil {
		return source["providerAssetId"].(string)
	}
	return ""
}

// Reports whether a schema-validated key has x, y < p and y^2 = x^3 + 7 (mod p).
func onSecp256k1(publicKey string) bool {
	der, err := hex.DecodeString(publicKey)
	if err != nil || len(der) != 23+65 {
		return false
	}
	// The schema pattern pins the 23-byte SPKI header; the uncompressed point 04 || x || y follows it.
	x, y := new(big.Int).SetBytes(der[24:56]), new(big.Int).SetBytes(der[56:])
	if x.Cmp(secp256k1P) >= 0 || y.Cmp(secp256k1P) >= 0 {
		return false
	}
	lhs, rhs := new(big.Int).Mul(y, y), new(big.Int).Mul(x, x)
	rhs.Mul(rhs, x).Add(rhs, big.NewInt(7))
	return lhs.Sub(lhs, rhs).Mod(lhs, secp256k1P).Sign() == 0
}

// Requires a schema-validated entry and its fields.
func validateIssuerSigning(entry Entry, fields map[string]any) error {
	kids, publicKeys := map[string]bool{}, map[string]bool{}
	for _, key := range entry.IssuerKeys {
		kid := strings.ToLower(key.Kid)
		if kids[kid] {
			return fmt.Errorf("duplicate issuer key kid %q", key.Kid)
		}
		if publicKeys[key.PublicKey] {
			return fmt.Errorf("issuer key %q repeats another key's publicKey", key.Kid)
		}
		if !onSecp256k1(key.PublicKey) {
			return fmt.Errorf("issuer key %q is not a point on secp256k1", key.Kid)
		}
		kids[kid], publicKeys[key.PublicKey] = true, true
	}
	source := findIssuerSource(fields)
	if source == nil {
		return nil
	}
	if !strings.HasPrefix(entry.IssuerFeedID, entry.Network+":") {
		return fmt.Errorf("issuer providerAssetId must start with %q", entry.Network+":")
	}
	// A signed price is the entry's own price, so it can't price another asset (reference) or be scaled (multiplier).
	if fields["pricing"].(map[string]any)["type"] != "direct" {
		return fmt.Errorf("an issuer source needs direct pricing")
	}
	if _, ok := source["multiplier"]; ok {
		return fmt.Errorf("an issuer source can't set multiplier")
	}
	if len(entry.IssuerKeys) == 0 {
		return fmt.Errorf("an issuer source needs issuerKeys")
	}
	return nil
}
