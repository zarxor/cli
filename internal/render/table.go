package render

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/zarxor/scripts/internal/tools"
)

type VersionRow struct {
	Tool             tools.Tool
	CurrentVersion   string
	CandidateVersion string
}

// VersionTable renders tool versions consistently for interactive and dry-run
// plans. A dash means the adapter could not provide that version.
func VersionTable(writer io.Writer, rows []VersionRow) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "TOOL\tCURRENT\tCANDIDATE"); err != nil {
		return err
	}
	for _, row := range rows {
		current := versionOrDash(row.CurrentVersion)
		candidate := versionOrDash(row.CandidateVersion)
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\n", row.Tool.Name, current, candidate); err != nil {
			return err
		}
	}
	return table.Flush()
}

func versionOrDash(version string) string {
	if version == "" {
		return "-"
	}
	return version
}
