package submission

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func entry(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"schemaVersion": 1, "network": "testnet", "assetKey": "BETH",
		"adminParty": "issuer::1220" + strings.Repeat("a", 64), "instrumentId": "BETH",
		"issuerName": "BitSafe", "name": "BETH", "symbol": "BETH", "decimals": 10,
		"logo": "assets/logo.png", "website": "https://example.com", "registryUrl": "https://example.com/registry",
		"wallet": map[string]any{"enabled": true, "visibleByDefault": true},
		"data":   map[string]any{"listed": true}, "pricing": map[string]any{"type": "none"},
	}
	if mutate != nil {
		mutate(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func logo(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 256, 256))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func otherIssuer(key string) func(map[string]any) {
	return func(m map[string]any) {
		m["assetKey"], m["instrumentId"], m["symbol"], m["name"] = key, key, key, key
		m["adminParty"] = "other::1220" + strings.Repeat("b", 64)
		m["issuerName"] = "Other"
	}
}

func baseRepo(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"README.md":                    {Data: []byte("readme")},
		"testnet/beth/info.json":       {Data: entry(t, nil)},
		"testnet/beth/assets/logo.png": {Data: logo(t)},
		"mainnet/legacy/logo.svg":      {Data: []byte("<svg/>")},
	}
}

func clone(m fstest.MapFS) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range m {
		copied := *v
		out[k] = &copied
	}
	return out
}

func hasFinding(findings []Finding, path, text string) bool {
	for _, f := range findings {
		if f.Path == path && strings.Contains(f.Message, text) {
			return true
		}
	}
	return false
}

func TestCurrentRepositoryPasses(t *testing.T) {
	root := os.DirFS("..")
	report, err := Check(root, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("existing entries fail the submission checks: %v", report.Errors)
	}
	if report.Entries == 0 {
		t.Fatal("found no entries")
	}
}

func TestReportsEveryInvalidEntry(t *testing.T) {
	head := baseRepo(t)
	head["testnet/aaa/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) {
		otherIssuer("AAA")(m)
		m["decimals"] = 11
	})}
	head["testnet/aaa/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	head["testnet/zzz/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) {
		otherIssuer("ZZZ")(m)
		m["network"] = "devnet"
	})}
	head["testnet/zzz/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	head["testnet/nologo/info.json"] = &fstest.MapFile{Data: entry(t, otherIssuer("NOLOGO"))}
	report, err := Check(head, nil)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"testnet/aaa/info.json":    "/decimals",
		"testnet/zzz/info.json":    "matching network",
		"testnet/nologo/info.json": "logo",
	} {
		if !hasFinding(report.Errors, path, want) {
			t.Errorf("no %q error for %s in %v", want, path, report.Errors)
		}
	}
	if hasFinding(report.Errors, "testnet/beth/info.json", "") {
		t.Errorf("valid entry reported: %v", report.Errors)
	}
}

func TestReportsSetAndTransitionErrors(t *testing.T) {
	base := baseRepo(t)
	head := clone(base)
	head["testnet/beth/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) { m["decimals"] = 8 })}
	head["testnet/beth2/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) {
		m["assetKey"] = "beth"
		m["instrumentId"] = "BETH2"
	})}
	head["testnet/beth2/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	report, err := Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report.Errors, "testnet/beth/info.json", "decimals") {
		t.Errorf("no decimals change error: %v", report.Errors)
	}
	if !hasFinding(report.Errors, "testnet/beth2/info.json", "duplicate asset key, identity, or path (with testnet/beth/info.json)") {
		t.Errorf("no duplicate key error: %v", report.Errors)
	}
}

func TestRejectsNewFilesThatAreNotEntriesOrLogos(t *testing.T) {
	base := baseRepo(t)
	head := clone(base)
	head["testnet/beth/zz_probe_test.go"] = &fstest.MapFile{Data: []byte("package beth")}
	head["testnet/beth/assets/extra.png"] = &fstest.MapFile{Data: logo(t)}
	head["testnet/new/info.json"] = &fstest.MapFile{Data: entry(t, otherIssuer("NEW"))}
	head["testnet/new/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	report, err := Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"testnet/beth/zz_probe_test.go", "testnet/beth/assets/extra.png"} {
		if !hasFinding(report.Errors, path, "not an info.json or its logo") {
			t.Errorf("no error for %s in %v", path, report.Errors)
		}
	}
	if len(report.Errors) != 2 {
		t.Errorf("want only the two stray files, got %v", report.Errors)
	}
	report, err = Check(head, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 0 {
		t.Errorf("without a base the file rule cannot tell new files from legacy ones: %v", report.Errors)
	}
	head["testnet/bad/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) { otherIssuer("BAD")(m); m["decimals"] = 11 })}
	head["testnet/bad/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	report, err = Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(report.Errors, "testnet/bad/assets/logo.png", "") {
		t.Errorf("the logo of an invalid entry was reported as a stray file: %v", report.Errors)
	}
}

func TestRejectsLocalHosts(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"https://example.com", true},
		{"https://registry.example.com/registry", true},
		{"https://127.0.0.1/registry", false},
		{"https://10.0.0.5", false},
		{"https://[::1]/registry", false},
		{"https://localhost", false},
		{"https://api.localhost", false},
		{"https://printer.local", false},
		{"https://registry.internal", false},
		{"https://intranet", false},
		{"https://127.1/registry", false},
		{"https://0x7f.0.0.1/registry", false},
		{"https://router.lan", false},
		{"https://nas.home.arpa", false},
	} {
		for _, field := range []string{"website", "registryUrl"} {
			t.Run(field+" "+tc.url, func(t *testing.T) {
				head := fstest.MapFS{
					"testnet/beth/info.json":       {Data: entry(t, func(m map[string]any) { m[field] = tc.url })},
					"testnet/beth/assets/logo.png": {Data: logo(t)},
				}
				report, err := Check(head, nil)
				if err != nil {
					t.Fatal(err)
				}
				if got := len(report.Errors) == 0; got != tc.valid {
					t.Fatalf("valid=%v, errors=%v", tc.valid, report.Errors)
				}
			})
		}
	}
}

func TestWarnsOnLookalikes(t *testing.T) {
	base := baseRepo(t)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		warn   bool
	}{
		{"same symbol other issuer", func(m map[string]any) { otherIssuer("XYZ")(m); m["symbol"] = "b-eth" }, true},
		{"same name other issuer", func(m map[string]any) { otherIssuer("XYZ")(m); m["name"] = "Beth" }, true},
		{"reserved word as symbol", func(m map[string]any) { otherIssuer("XYZ")(m); m["symbol"] = "CC" }, true},
		{"other network", func(m map[string]any) {
			otherIssuer("XYZ")(m)
			m["symbol"] = "BETH"
			m["network"] = "devnet"
		}, false},
		{"distinct", otherIssuer("XYZ"), false},
		{"same issuer", func(m map[string]any) { m["assetKey"], m["instrumentId"] = "XYZ", "XYZ" }, false},
		{"Cyrillic symbol", func(m map[string]any) { otherIssuer("XYZ")(m); m["symbol"] = "USD\u0421" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := clone(base)
			data := entry(t, tc.mutate)
			var meta struct{ Network string }
			if err := json.Unmarshal(data, &meta); err != nil {
				t.Fatal(err)
			}
			head[meta.Network+"/xyz/info.json"] = &fstest.MapFile{Data: data}
			head[meta.Network+"/xyz/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
			report, err := Check(head, base)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Errors) != 0 {
				t.Fatalf("unexpected errors: %v", report.Errors)
			}
			if got := len(report.Warnings) > 0; got != tc.warn {
				t.Fatalf("warn=%v, warnings=%v", tc.warn, report.Warnings)
			}
		})
	}
}

func TestClassifiesChanges(t *testing.T) {
	base := baseRepo(t)
	head := clone(base)
	head["README.md"] = &fstest.MapFile{Data: []byte("changed")}
	head[".git"] = &fstest.MapFile{Data: []byte("gitdir: /elsewhere")}
	head[".github/workflows/validate.yml"] = &fstest.MapFile{Data: []byte("on: push")}
	head["testnet/beth/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) { m["name"] = "Bitsafe ETH" })}
	head["testnet/new/info.json"] = &fstest.MapFile{Data: entry(t, otherIssuer("NEW"))}
	head["testnet/new/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	delete(head, "mainnet/legacy/logo.svg")
	report, err := Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Added) != 1 || report.Added[0].Path != "testnet/new/info.json" || report.Added[0].AssetKey != "NEW" {
		t.Errorf("added: %+v", report.Added)
	}
	if strings.Join(report.Modified, ",") != "testnet/beth/info.json" {
		t.Errorf("modified: %v", report.Modified)
	}
	if strings.Join(report.Deleted, ",") != "mainnet/legacy/logo.svg" {
		t.Errorf("deleted: %v", report.Deleted)
	}
	if strings.Join(report.NonData, ",") != ".github/workflows/validate.yml,README.md" {
		t.Errorf("non-data: %v", report.NonData)
	}
	summary := Markdown(report)
	for _, want := range []string{"NEW", "testnet/beth/info.json", ".github/workflows/validate.yml", "mainnet/legacy/logo.svg"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary misses %q:\n%s", want, summary)
		}
	}
}

func TestWarnsWhenAnExistingEntryCopiesAnotherIssuer(t *testing.T) {
	base := baseRepo(t)
	base["testnet/xyz/info.json"] = &fstest.MapFile{Data: entry(t, otherIssuer("XYZ"))}
	base["testnet/xyz/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	head := clone(base)
	head["testnet/xyz/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) { otherIssuer("XYZ")(m); m["symbol"] = "BETH" })}
	report, err := Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report.Warnings, "testnet/xyz/info.json", "symbol \"BETH\" looks like testnet/beth/info.json") {
		t.Fatalf("no warning for a modified entry: %v", report.Warnings)
	}
}

func TestSummaryEscapesSubmittedText(t *testing.T) {
	base := baseRepo(t)
	head := clone(base)
	head["testnet/xyz/info.json"] = &fstest.MapFile{Data: entry(t, func(m map[string]any) {
		otherIssuer("XYZ")(m)
		m["name"] = "![ok](https://example.com/pass.png) | <b>x</b>"
	})}
	head["testnet/xyz/assets/logo.png"] = &fstest.MapFile{Data: logo(t)}
	report, err := Check(head, base)
	if err != nil {
		t.Fatal(err)
	}
	summary := Markdown(report)
	for _, raw := range []string{"![ok](", "<b>", "| <b>", "x</b>"} {
		if strings.Contains(summary, raw) {
			t.Errorf("summary keeps %q:\n%s", raw, summary)
		}
	}
	if !strings.Contains(summary, "\\!\\[ok\\]") {
		t.Errorf("summary drops the escaped name:\n%s", summary)
	}
}
