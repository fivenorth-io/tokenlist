// Package submission holds CI-only rules. Loop Wallet and Loop Data do not import it, so a rule
// here never stops an app from loading a network.
package submission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/fivenorth-io/tokenlist/validator"
)

type Finding struct {
	Path    string
	Message string
}

type Asset struct {
	Path, Network, AssetKey, Symbol, Name, IssuerName, Pricing string
	Sources                                                    []string
	WalletEnabled, WalletVisible, Listed                       bool
}

type Report struct {
	Entries                    int
	Errors                     []Finding
	Warnings                   []Finding
	Added                      []Asset
	Modified, Deleted, NonData []string
}

var networks = map[string]bool{"mainnet": true, "devnet": true, "testnet": true}

type document struct {
	Symbol      string `json:"symbol"`
	Name        string `json:"name"`
	Website     string `json:"website"`
	RegistryURL string `json:"registryUrl"`
	Wallet      struct {
		Enabled          bool `json:"enabled"`
		VisibleByDefault bool `json:"visibleByDefault"`
	} `json:"wallet"`
	Data struct {
		Listed bool `json:"listed"`
	} `json:"data"`
	Pricing struct {
		Type    string `json:"type"`
		Sources []struct {
			Provider        string `json:"provider"`
			ProviderAssetID string `json:"providerAssetId"`
		} `json:"sources"`
	} `json:"pricing"`
}

type checked struct {
	entry validator.Entry
	doc   document
}

// Check reports every problem in head. A nil base skips the rules that need a diff.
func Check(head, base fs.FS) (Report, error) {
	var report Report
	v, err := validator.New()
	if err != nil {
		return report, err
	}
	headFiles, err := readFiles(head)
	if err != nil {
		return report, err
	}
	var entries []checked
	logos := map[string]bool{}
	for _, name := range sortedKeys(headFiles) {
		file := headFiles[name]
		if file.symlink {
			report.fail(name, "symlinks are not allowed")
			continue
		}
		if path.Base(name) != "info.json" || !networks[topDir(name)] {
			continue
		}
		report.Entries++
		var ref struct{ Logo string }
		if json.Unmarshal(file.data, &ref) == nil && ref.Logo != "" {
			logos[path.Join(path.Dir(name), ref.Logo)] = true
		}
		entry, err := v.Validate(name, file.data)
		if err != nil {
			report.fail(name, trimPath(name, err))
			continue
		}
		if err := validator.ValidateLogo(head, entry); err != nil {
			report.fail(name, trimPath(name, err))
		}
		var doc document
		if err := json.Unmarshal(file.data, &doc); err != nil {
			report.fail(name, err.Error())
			continue
		}
		report.checkHosts(name, doc)
		entries = append(entries, checked{entry, doc})
	}
	report.checkSet(entries)
	if base == nil {
		return report, nil
	}
	baseFiles, err := readFiles(base)
	if err != nil {
		return report, fmt.Errorf("base: %w", err)
	}
	previous, err := validator.ReadIdentities(base)
	if err != nil {
		return report, fmt.Errorf("base registry: %w", err)
	}
	report.checkTransition(previous, entries)
	report.classify(headFiles, baseFiles, entries, logos)
	return report, nil
}

func (r *Report) fail(path, message string) { r.Errors = append(r.Errors, Finding{path, message}) }
func (r *Report) warn(path, message string) { r.Warnings = append(r.Warnings, Finding{path, message}) }

func (r *Report) checkHosts(name string, doc document) {
	for _, field := range []struct{ key, value string }{{"website", doc.Website}, {"registryUrl", doc.RegistryURL}} {
		u, err := url.Parse(field.value)
		if err != nil {
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		labels := strings.Split(host, ".")
		last := labels[len(labels)-1]
		local := net.ParseIP(host) != nil || len(labels) < 2 || host == "localhost" ||
			strings.Trim(last, "0123456789") == "" || strings.HasPrefix(last, "0x")
		for _, suffix := range []string{".localhost", ".local", ".internal", ".arpa", ".lan", ".home", ".corp"} {
			local = local || strings.HasSuffix(host, suffix)
		}
		if local {
			r.fail(name, fmt.Sprintf("%s host %q must be a public domain name", field.key, host))
		}
	}
}

func (r *Report) checkSet(entries []checked) {
	for i := range entries {
		var pair []validator.Entry
		for j := 0; j < i; j++ {
			pair = append(pair[:0], entries[j].entry, entries[i].entry)
			if err := validator.ValidateSet(pair); err != nil {
				r.fail(entries[i].entry.Path, fmt.Sprintf("%s (with %s)", trimPath(entries[i].entry.Path, err), entries[j].entry.Path))
				break
			}
		}
	}
}

func (r *Report) checkTransition(previous []validator.Entry, entries []checked) {
	current := make([]validator.Entry, len(entries))
	for i, c := range entries {
		current[i] = c.entry
	}
	for _, old := range previous {
		if err := validator.ValidateTransition([]validator.Entry{old}, current); err != nil {
			r.fail(old.Path, trimPath(old.Path, err))
		}
	}
}

func (r *Report) classify(headFiles, baseFiles map[string]file, entries []checked, logos map[string]bool) {
	byPath := map[string]checked{}
	for _, c := range entries {
		byPath[c.entry.Path] = c
	}
	for _, name := range sortedKeys(headFiles) {
		old, existed := baseFiles[name]
		if existed && bytes.Equal(old.data, headFiles[name].data) {
			continue
		}
		data := networks[topDir(name)]
		switch {
		case !data:
			r.NonData = append(r.NonData, name)
		case existed:
			r.Modified = append(r.Modified, name)
			if c, ok := byPath[name]; ok {
				r.checkLookalike(c, entries)
			}
		case path.Base(name) == "info.json":
			if c, ok := byPath[name]; ok {
				r.Added = append(r.Added, asset(c))
				r.checkLookalike(c, entries)
			}
		case !logos[name]:
			r.fail(name, "new file is not an info.json or its logo")
		}
	}
	for _, name := range sortedKeys(baseFiles) {
		if _, ok := headFiles[name]; ok {
			continue
		}
		if networks[topDir(name)] {
			r.Deleted = append(r.Deleted, name)
		} else {
			r.NonData = append(r.NonData, name)
		}
	}
	sort.Strings(r.NonData)
}

func (r *Report) checkLookalike(c checked, entries []checked) {
	for _, field := range []struct{ key, value string }{{"symbol", c.doc.Symbol}, {"name", c.doc.Name}} {
		if strings.IndexFunc(field.value, func(r rune) bool { return r > '~' }) >= 0 {
			r.warn(c.entry.Path, fmt.Sprintf("%s %q has characters outside ASCII", field.key, field.value))
		}
	}
	symbol, name := fold(c.doc.Symbol), fold(c.doc.Name)
	if validator.IsReservedKey(symbol) {
		r.warn(c.entry.Path, fmt.Sprintf("symbol %q matches a reserved Loop Data market", c.doc.Symbol))
	}
	for _, other := range entries {
		if other.entry.Path == c.entry.Path || other.entry.Network != c.entry.Network || other.entry.AdminParty == c.entry.AdminParty {
			continue
		}
		if symbol != "" && symbol == fold(other.doc.Symbol) {
			r.warn(c.entry.Path, fmt.Sprintf("symbol %q looks like %s from another issuer", c.doc.Symbol, other.entry.Path))
		}
		if name != "" && name == fold(other.doc.Name) {
			r.warn(c.entry.Path, fmt.Sprintf("name %q looks like %s from another issuer", c.doc.Name, other.entry.Path))
		}
	}
}

func asset(c checked) Asset {
	a := Asset{
		Path: c.entry.Path, Network: c.entry.Network, AssetKey: c.entry.AssetKey, Symbol: c.doc.Symbol,
		Name: c.doc.Name, IssuerName: c.entry.IssuerName, Pricing: c.doc.Pricing.Type,
		WalletEnabled: c.doc.Wallet.Enabled, WalletVisible: c.doc.Wallet.VisibleByDefault, Listed: c.doc.Data.Listed,
	}
	for _, s := range c.doc.Pricing.Sources {
		a.Sources = append(a.Sources, s.Provider+":"+s.ProviderAssetID)
	}
	return a
}

func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type file struct {
	data    []byte
	symlink bool
}

func readFiles(root fs.FS) (map[string]file, error) {
	files := map[string]file{}
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == ".git" && d.IsDir() {
			return fs.SkipDir
		}
		if d.IsDir() || name == ".git" {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			files[name] = file{symlink: true}
			return nil
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		files[name] = file{data: data}
		return nil
	})
	return files, err
}

func topDir(name string) string {
	top, _, _ := strings.Cut(name, "/")
	return top
}

func trimPath(name string, err error) string {
	return strings.TrimPrefix(err.Error(), name+": ")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
