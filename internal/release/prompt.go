package release

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrCancelled indicates that the user declined or ended the release flow.
var ErrCancelled = errors.New("release cancelled")

// Prompter owns the shared buffered input used by the interactive release.
type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

// NewPrompter creates an interactive release prompter.
func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out}
}

// SelectVersion asks the user to select a calculated or custom stable version.
func (p *Prompter) SelectVersion(current Version, existing map[string]struct{}) (Version, error) {
	patch, err := current.Next(BumpPatch)
	if err != nil {
		return Version{}, err
	}
	minor, err := current.Next(BumpMinor)
	if err != nil {
		return Version{}, err
	}
	major, err := current.Next(BumpMajor)
	if err != nil {
		return Version{}, err
	}

	for {
		fmt.Fprintf(p.out, `Current version: %s

Select the next version:
  1. Patch   %s
  2. Minor   %s
  3. Major   %s
  4. Custom
  5. Cancel
Selection: `, current, patch, minor, major)

		selection, err := p.readLine()
		if err != nil {
			return Version{}, err
		}
		switch selection {
		case "1":
			return patch, nil
		case "2":
			return minor, nil
		case "3":
			return major, nil
		case "4":
			fmt.Fprint(p.out, "Custom version: ")
			input, err := p.readLine()
			if err != nil {
				return Version{}, err
			}
			custom, err := ParseVersion(input)
			if err != nil {
				fmt.Fprintln(p.out, err)
				continue
			}
			if custom.Compare(current) <= 0 {
				fmt.Fprintf(p.out, "version must be greater than %s\n", current)
				continue
			}
			if _, found := existing[custom.String()]; found {
				fmt.Fprintf(p.out, "tag %s already exists\n", custom)
				continue
			}
			return custom, nil
		case "5":
			return Version{}, ErrCancelled
		default:
			fmt.Fprintln(p.out, "Select 1, 2, 3, 4, or 5.")
		}
	}
}

// Confirm asks for explicit permission to begin remote mutations.
func (p *Prompter) Confirm() (bool, error) {
	fmt.Fprint(p.out, "Publish this release now? [y/N]: ")
	answer, err := p.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func (p *Prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read release selection: %w", err)
	}
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return "", ErrCancelled
	}
	return strings.TrimSpace(line), nil
}
