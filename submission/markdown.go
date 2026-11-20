package submission

import (
	"fmt"
	"strings"
)

func Markdown(r Report) string {
	var b strings.Builder
	b.WriteString("## Tokenlist submission check\n\n")
	fmt.Fprintf(&b, "%d entries checked, %d errors, %d warnings.\n\n", r.Entries, len(r.Errors), len(r.Warnings))
	findings(&b, "Errors", r.Errors)
	findings(&b, "Warnings", r.Warnings)
	if len(r.Added) > 0 {
		b.WriteString("### New assets\n\n")
		b.WriteString("| Path | Network | Key | Symbol | Name | Issuer | Pricing | Sources | Wallet enabled | Visible by default | Listed |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, a := range r.Added {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %v | %v | %v |\n",
				EscapeUntrustedCell(a.Path), EscapeUntrustedCell(a.Network), EscapeUntrustedCell(a.AssetKey), EscapeUntrustedCell(a.Symbol), EscapeUntrustedCell(a.Name), EscapeUntrustedCell(a.IssuerName),
				EscapeUntrustedCell(a.Pricing), EscapeUntrustedCell(strings.Join(a.Sources, ", ")), a.WalletEnabled, a.WalletVisible, a.Listed)
		}
		b.WriteString("\n")
	}
	paths(&b, "Changed existing files", r.Modified)
	paths(&b, "Deleted files", r.Deleted)
	paths(&b, "Changes outside asset folders (need a code owner)", r.NonData)
	return b.String()
}

func findings(b *strings.Builder, title string, list []Finding) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	for _, f := range list {
		fmt.Fprintf(b, "- `%s`: %s\n", EscapeUntrustedCell(f.Path), EscapeUntrustedCell(f.Message))
	}
	b.WriteString("\n")
}

func paths(b *strings.Builder, title string, list []string) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(b, "### %s\n\n", title)
	for _, p := range list {
		fmt.Fprintf(b, "- `%s`\n", EscapeUntrustedCell(p))
	}
	b.WriteString("\n")
}

func EscapeUntrustedCell(s string) string {
	r := strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ", "`", "'", "<", "&lt;", ">", "&gt;",
		"[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "!", "\\!")
	return r.Replace(s)
}
