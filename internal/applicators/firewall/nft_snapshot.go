package firewall

import (
	"bytes"
	"strings"
)

// OmitForeignOwnedNFTTables drops nftables tables annotated with
// `# progname <owner>` (for example firewalld). Remotr-managed snapshots must
// not capture foreign-owned tables: restoring them while the owner daemon is
// active fails with "Operation not permitted".
func OmitForeignOwnedNFTTables(ruleset []byte) []byte {
	lines := bytes.Split(ruleset, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	depth := 0
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if depth == 0 && strings.HasPrefix(trimmed, "table ") {
			if strings.Contains(trimmed, "# progname ") {
				skipping = true
			}
		}
		open := bytes.Count(line, []byte("{"))
		close := bytes.Count(line, []byte("}"))
		if !skipping {
			out = append(out, line)
		}
		depth += open - close
		if depth < 0 {
			depth = 0
		}
		if skipping && depth == 0 {
			skipping = false
		}
	}
	return bytes.Join(out, []byte("\n"))
}
