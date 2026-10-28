package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesCurrentButOnlyReadsBaseIdentities(t *testing.T) {
	base, current := t.TempDir(), t.TempDir()
	identity := `"network":"testnet","assetKey":"BETH","adminParty":"issuer::1220` + strings.Repeat("a", 64) + `","instrumentId":"BETH"`
	old := `{` + identity + `,"decimals":10,"website":"http://old.example.com","oldField":true}`
	valid := `{` + identity + `,"schemaVersion":1,"issuerName":"Issuer","name":"BETH","symbol":"BETH","decimals":10,"logo":"logo.png","website":"https://example.com","registryUrl":"https://example.com/registry","wallet":{"enabled":true,"visibleByDefault":true},"data":{"listed":true},"pricing":{"type":"none"}}`
	write := func(root, name, content string) {
		t.Helper()
		file := filepath.Join(root, "testnet", "beth", name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(base, "info.json", old)
	write(current, "info.json", valid)
	write(current, "logo.png", "logo")
	if err := run(current, base); err != nil {
		t.Fatal(err)
	}
	write(current, "info.json", old)
	if err := run(current, base); err == nil {
		t.Fatal("current metadata must still satisfy the full schema")
	}
	write(current, "info.json", strings.Replace(valid, `"assetKey":"BETH"`, `"assetKey":"RENAMED"`, 1))
	if err := run(current, base); err == nil {
		t.Fatal("accepted rename against base identity")
	}
	write(current, "info.json", strings.Replace(valid, `"decimals":10`, `"decimals":6`, 1))
	if err := run(current, base); err == nil {
		t.Fatal("accepted changed decimals against base metadata")
	}
}
