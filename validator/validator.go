// Package validator validates Loop tokenlist metadata without fetching network resources.
package validator

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strings"

	"github.com/fivenorth-io/tokenlist/schema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type Entry struct {
	Path         string
	Network      string
	AssetKey     string
	AdminParty   string
	InstrumentID string
	IssuerName   string
	RegistryURL  string
	Logo         string
}

type Validator struct{ schema *jsonschema.Schema }

const maxLogoBytes = 2_000_000

func New() (*Validator, error) {
	compiled, err := jsonschema.CompileString("asset.schema.json", schema.Asset)
	if err != nil {
		return nil, err
	}
	return &Validator{schema: compiled}, nil
}

func safePath(p string) bool {
	return fs.ValidPath(p) && p != "." && !strings.ContainsAny(p, "\\:%?#\x00")
}

// Validate checks one metadata document. filename is relative to the repo root.
func (v *Validator) Validate(filename string, data []byte) (Entry, error) {
	var entry Entry
	if len(data) > 64<<10 {
		return entry, fmt.Errorf("%s: metadata exceeds 64 KiB", filename)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return entry, fmt.Errorf("%s: %w", filename, err)
	}
	if err := v.schema.Validate(value); err != nil {
		return entry, fmt.Errorf("%s: %w", filename, err)
	}
	// Match the integer decoding used by registry consumers, not just JSON Schema's numeric equality.
	var integers struct {
		SchemaVersion int    `json:"schemaVersion"`
		Decimals      *int64 `json:"decimals"`
	}
	if err := json.Unmarshal(data, &integers); err != nil {
		return entry, fmt.Errorf("%s: %w", filename, err)
	}
	fields := value.(map[string]any)
	entry = Entry{Path: filename, Network: fields["network"].(string), AssetKey: fields["assetKey"].(string), AdminParty: fields["adminParty"].(string), InstrumentID: fields["instrumentId"].(string), IssuerName: fields["issuerName"].(string), RegistryURL: fields["registryUrl"].(string), Logo: fields["logo"].(string)}
	parts := strings.Split(filename, "/")
	if !safePath(filename) || len(parts) != 3 || parts[0] != entry.Network || parts[2] != "info.json" {
		return entry, fmt.Errorf("%s: expected <network>/<asset>/info.json matching network", filename)
	}
	if !safePath(entry.Logo) {
		return entry, fmt.Errorf("%s: unsafe relative logo path", filename)
	}
	switch strings.ToLower(path.Ext(entry.Logo)) {
	case ".png", ".svg", ".jpg", ".jpeg":
	default:
		return entry, fmt.Errorf("%s: unsupported logo extension", filename)
	}
	for _, key := range []string{"website", "registryUrl"} {
		u, err := url.Parse(fields[key].(string))
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return entry, fmt.Errorf("%s: invalid %s", filename, key)
		}
	}
	return entry, nil
}

// ValidateSet checks uniqueness and issuer consistency within each network.
func ValidateSet(entries []Entry) error {
	keys, identities, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	issuers, urls := map[string]string{}, map[string]string{}
	for _, e := range entries {
		admin := e.Network + "\x00" + e.AdminParty
		key, identity := e.Network+"\x00"+e.AssetKey, admin+"\x00"+e.InstrumentID
		if keys[key] || identities[identity] || paths[e.Path] {
			return fmt.Errorf("%s: duplicate asset key, identity, or path", e.Path)
		}
		keys[key], identities[identity], paths[e.Path] = true, true, true
		if name, ok := issuers[admin]; ok && name != e.IssuerName {
			return fmt.Errorf("%s: inconsistent issuerName for admin", e.Path)
		}
		if endpoint, ok := urls[admin]; ok && endpoint != e.RegistryURL {
			return fmt.Errorf("%s: inconsistent registryUrl for admin", e.Path)
		}
		issuers[admin], urls[admin] = e.IssuerName, e.RegistryURL
	}
	return nil
}

// ValidateTransition preserves public keys and on-chain identities during migration.
func ValidateTransition(previous, current []Entry) error {
	identities := map[string]Entry{}
	for _, e := range current {
		identities[e.Network+"\x00"+e.AdminParty+"\x00"+e.InstrumentID] = e
	}
	for _, e := range previous {
		next, ok := identities[e.Network+"\x00"+e.AdminParty+"\x00"+e.InstrumentID]
		if !ok || next.AssetKey != e.AssetKey {
			return fmt.Errorf("%s: asset removed, identity changed, or assetKey renamed; disable instead", e.Path)
		}
	}
	return nil
}

// ReadIdentities reads the base revision without applying current metadata or logo rules.
func ReadIdentities(root fs.FS) ([]Entry, error) {
	var entries []Entry
	err := fs.WalkDir(root, ".", func(filename string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filename == ".git" {
			return fs.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlinks are not allowed", filename)
		}
		if d.IsDir() || d.Name() != "info.json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return fmt.Errorf("%s: invalid metadata file", filename)
		}
		data, err := fs.ReadFile(root, filename)
		if err != nil {
			return err
		}
		var identity struct {
			Network      string `json:"network"`
			AssetKey     string `json:"assetKey"`
			AdminParty   string `json:"adminParty"`
			InstrumentID string `json:"instrumentId"`
		}
		if err := json.Unmarshal(data, &identity); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		parts := strings.Split(filename, "/")
		if !safePath(filename) || len(parts) != 3 || parts[0] != identity.Network {
			return fmt.Errorf("%s: invalid identity path", filename)
		}
		for _, field := range []string{identity.Network, identity.AssetKey, identity.AdminParty, identity.InstrumentID} {
			if strings.TrimSpace(field) == "" || strings.ContainsRune(field, 0) {
				return fmt.Errorf("%s: missing or invalid identity field", filename)
			}
		}
		entries = append(entries, Entry{Path: filename, Network: identity.Network, AssetKey: identity.AssetKey, AdminParty: identity.AdminParty, InstrumentID: identity.InstrumentID})
		return nil
	})
	return entries, err
}

// ValidateRepository checks all info.json files and referenced logos. Legacy logo-only folders are allowed.
func (v *Validator) ValidateRepository(root fs.FS) ([]Entry, error) {
	var entries []Entry
	err := fs.WalkDir(root, ".", func(filename string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filename == ".git" {
			return fs.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlinks are not allowed", filename)
		}
		// Include legacy logos that do not yet have an info.json reference.
		if !d.IsDir() {
			switch strings.ToLower(path.Ext(filename)) {
			case ".png", ".svg", ".jpg", ".jpeg":
				info, err := d.Info()
				if err != nil {
					return err
				}
				if info.Size() > maxLogoBytes {
					return fmt.Errorf("%s: logo exceeds 2 MB", filename)
				}
			}
		}
		if d.IsDir() || d.Name() != "info.json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return fmt.Errorf("%s: invalid metadata file", filename)
		}
		data, err := fs.ReadFile(root, filename)
		if err != nil {
			return err
		}
		entry, err := v.Validate(filename, data)
		if err != nil {
			return err
		}
		logo, err := fs.Stat(root, path.Join(path.Dir(filename), entry.Logo))
		if err != nil {
			return fmt.Errorf("%s: logo: %w", filename, err)
		}
		if !logo.Mode().IsRegular() || logo.Size() == 0 {
			return fmt.Errorf("%s: logo must be a nonempty regular file", filename)
		}
		if logo.Size() > maxLogoBytes {
			return fmt.Errorf("%s: logo exceeds 2 MB", filename)
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, ValidateSet(entries)
}
