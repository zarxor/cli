// Package render provides portable terminal rendering and selection.
package render

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/zarxor/scripts/internal/tools"
)

type SelectionUI interface {
	Select(ctx context.Context, items []Item) ([]tools.ToolID, error)
}

type Item struct {
	Tool     tools.Tool
	Label    string
	Selected bool
}

// NumberedSelection is a terminal-independent selector. Entered numbers
// toggle the displayed defaults; an empty line accepts them unchanged.
type NumberedSelection struct {
	reader io.Reader
	writer io.Writer
}

func NewNumberedSelection(reader io.Reader, writer io.Writer) *NumberedSelection {
	return &NumberedSelection{reader: reader, writer: writer}
}

func (s *NumberedSelection) Select(ctx context.Context, items []Item) ([]tools.ToolID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	selected := make([]bool, len(items))
	table := tabwriter.NewWriter(s.writer, 0, 4, 2, ' ', 0)
	for i, item := range items {
		selected[i] = item.Selected
		mark := "[ ]"
		if item.Selected {
			mark = "[x]"
		}
		label := item.Label
		if label == "" {
			label = item.Tool.Name
		}
		if _, err := fmt.Fprintf(table, "%d\t%s\t%s\n", i+1, mark, label); err != nil {
			return nil, err
		}
	}
	if err := table.Flush(); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprint(s.writer, "Toggle numbers (comma-separated), or press Enter to accept defaults: "); err != nil {
		return nil, err
	}

	line, err := bufio.NewReader(s.reader).ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	toggled := make(map[int]struct{})
	for _, value := range strings.FieldsFunc(strings.TrimSpace(line), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		number, parseErr := strconv.Atoi(value)
		if parseErr != nil || number < 1 || number > len(items) {
			return nil, fmt.Errorf("invalid selection %q: enter numbers from 1 to %d", value, len(items))
		}
		if _, exists := toggled[number-1]; exists {
			continue
		}
		toggled[number-1] = struct{}{}
		selected[number-1] = !selected[number-1]
	}

	ids := make([]tools.ToolID, 0, len(items))
	for i, item := range items {
		if selected[i] {
			ids = append(ids, item.Tool.ID)
		}
	}
	return ids, nil
}

var _ SelectionUI = (*NumberedSelection)(nil)
