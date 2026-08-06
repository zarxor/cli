package render

import (
	"bytes"
	"fmt"
	"io"
	"strings"
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
	return NewPlainRenderer(writer).VersionTable(rows)
}

func (r *Renderer) VersionTable(rows []VersionRow) error {
	var output bytes.Buffer
	table := tabwriter.NewWriter(&output, 0, 4, 2, ' ', 0)
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
	if err := table.Flush(); err != nil {
		return err
	}
	value := output.String()
	value = strings.Replace(value, "TOOL", r.theme.Accent("TOOL"), 1)
	value = strings.Replace(value, "CURRENT", r.theme.Accent("CURRENT"), 1)
	value = strings.Replace(value, "CANDIDATE", r.theme.Accent("CANDIDATE"), 1)
	_, err := io.WriteString(r.writer, value)
	return err
}

func versionOrDash(version string) string {
	if version == "" {
		return "-"
	}
	return version
}
