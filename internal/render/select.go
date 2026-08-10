// Package render provides portable terminal rendering and selection.
package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/zarxor/cli/internal/tools"
)

var (
	ErrCancelled              = errors.New("selection cancelled")
	ErrInteractiveUnavailable = errors.New("interactive selection unavailable")
)

type SelectionUI interface {
	Select(ctx context.Context, items []Item) ([]SelectionID, error)
}

// SelectionID is the stable identifier returned by a selection control. It is
// kept compatible with the tool catalog ID type while the renderer itself
// only uses the identifier and display fields.
type SelectionID = tools.ToolID

type Item struct {
	// Tool is retained for compatibility with existing tool selection callers.
	// New resource integrations should provide ID and Name instead.
	Tool     tools.Tool
	ID       SelectionID
	Name     string
	Group    string
	Label    string
	Selected bool
	Disabled bool
}

func itemID(item Item) SelectionID {
	if item.ID != "" {
		return item.ID
	}
	return SelectionID(item.Tool.ID)
}

func itemName(item Item) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Tool.Name
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

func (s *NumberedSelection) Select(ctx context.Context, items []Item) ([]SelectionID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	selected := make([]bool, len(items))
	available := make([]int, 0, len(items))
	installed := make([]int, 0, len(items))
	for i, item := range items {
		selected[i] = item.Selected && !item.Disabled
		if item.Disabled {
			installed = append(installed, i)
		} else {
			available = append(available, i)
		}
	}

	table := tabwriter.NewWriter(s.writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "Available to install"); err != nil {
		return nil, err
	}
	group := ""
	for number, itemIndex := range available {
		item := items[itemIndex]
		if item.Group != "" && item.Group != group {
			if group != "" {
				if _, err := fmt.Fprintln(table); err != nil {
					return nil, err
				}
			}
			if _, err := fmt.Fprintln(table, item.Group); err != nil {
				return nil, err
			}
			group = item.Group
		}
		mark := "[ ]"
		if item.Selected {
			mark = "[x]"
		}
		label := item.Label
		if label == "" {
			label = itemName(item)
		}
		if _, err := fmt.Fprintf(table, "%d\t%s\t%s\n", number+1, mark, label); err != nil {
			return nil, err
		}
	}
	if len(installed) > 0 {
		if _, err := fmt.Fprintln(table, "\nAlready installed"); err != nil {
			return nil, err
		}
		group = ""
		for _, itemIndex := range installed {
			item := items[itemIndex]
			if item.Group != "" && item.Group != group {
				if group != "" {
					if _, err := fmt.Fprintln(table); err != nil {
						return nil, err
					}
				}
				if _, err := fmt.Fprintln(table, item.Group); err != nil {
					return nil, err
				}
				group = item.Group
			}
			label := item.Label
			if label == "" {
				label = itemName(item)
			}
			if _, err := fmt.Fprintf(table, "\t[-]\t%s\n", label); err != nil {
				return nil, err
			}
		}
	}
	if err := table.Flush(); err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return nil, nil
	}
	if _, err := fmt.Fprint(s.writer, "Toggle numbers (comma-separated), or press Enter to accept defaults: "); err != nil {
		return nil, err
	}

	line, err := readSelectionLine(s.reader)
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("interactive selection input closed: %w", err)
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	toggled := make(map[int]struct{})
	values, err := selectionValues(line)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		number, parseErr := strconv.Atoi(value)
		if parseErr != nil || number < 1 || number > len(available) {
			return nil, fmt.Errorf("invalid selection %q: enter numbers from 1 to %d", value, len(available))
		}
		itemIndex := available[number-1]
		if _, exists := toggled[itemIndex]; exists {
			continue
		}
		toggled[itemIndex] = struct{}{}
		selected[itemIndex] = !selected[itemIndex]
	}

	ids := make([]SelectionID, 0, len(items))
	for i, item := range items {
		if selected[i] {
			ids = append(ids, itemID(item))
		}
	}
	return ids, nil
}

func readSelectionLine(reader io.Reader) (string, error) {
	var line []byte
	buffer := []byte{0}
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			line = append(line, buffer[0])
			if buffer[0] == '\n' {
				return string(line), nil
			}
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return string(line), io.EOF
			}
			return string(line), err
		}
	}
}

func selectionValues(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	var values []string
	for _, group := range strings.Split(line, ",") {
		fields := strings.Fields(group)
		if len(fields) == 0 {
			return nil, fmt.Errorf("invalid selection %q: use comma-separated numbers", line)
		}
		values = append(values, fields...)
	}
	return values, nil
}

var _ SelectionUI = (*NumberedSelection)(nil)
