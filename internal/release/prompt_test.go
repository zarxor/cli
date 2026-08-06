package release

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSelectVersionOffersCalculatedChoices(t *testing.T) {
	out := new(bytes.Buffer)
	p := NewPrompter(strings.NewReader("2\n"), out)
	got, err := p.SelectVersion(Version{Major: 1, Minor: 2, Patch: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "v1.3.0" {
		t.Fatalf("SelectVersion() = %s, want v1.3.0", got)
	}
	for _, text := range []string{
		"Current version: v1.2.3",
		"Patch   v1.2.4",
		"Minor   v1.3.0",
		"Major   v2.0.0",
		"Custom",
		"Cancel",
	} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("menu missing %q:\n%s", text, out)
		}
	}
}

func TestSelectVersionCalculatedChoices(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "1\n", want: "v1.2.4"},
		{input: "2\n", want: "v1.3.0"},
		{input: "3\n", want: "v2.0.0"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			p := NewPrompter(strings.NewReader(test.input), io.Discard)
			got, err := p.SelectVersion(Version{Major: 1, Minor: 2, Patch: 3}, nil)
			if err != nil || got.String() != test.want {
				t.Fatalf("SelectVersion() = %s, %v; want %s", got, err, test.want)
			}
		})
	}
}

func TestSelectVersionCustomMustIncreaseAndBeUnused(t *testing.T) {
	out := new(bytes.Buffer)
	p := NewPrompter(
		strings.NewReader("4\nv1.2.3\n4\nv1.2.4\n4\nv1.2.5\n"),
		out,
	)
	got, err := p.SelectVersion(
		Version{Major: 1, Minor: 2, Patch: 3},
		map[string]struct{}{"v1.2.4": {}},
	)
	if err != nil || got.String() != "v1.2.5" {
		t.Fatalf("SelectVersion() = %s, %v; want v1.2.5", got, err)
	}
	if !strings.Contains(out.String(), "must be greater than v1.2.3") {
		t.Fatalf("missing increasing-version error:\n%s", out)
	}
	if !strings.Contains(out.String(), "tag v1.2.4 already exists") {
		t.Fatalf("missing duplicate-tag error:\n%s", out)
	}
}

func TestSelectVersionRepromptsAfterInvalidInput(t *testing.T) {
	out := new(bytes.Buffer)
	p := NewPrompter(strings.NewReader("wrong\n4\nv1.2.3-rc.1\n1\n"), out)
	got, err := p.SelectVersion(Version{Major: 1, Minor: 2, Patch: 3}, nil)
	if err != nil || got.String() != "v1.2.4" {
		t.Fatalf("SelectVersion() = %s, %v", got, err)
	}
	if !strings.Contains(out.String(), "Select 1, 2, 3, 4, or 5") {
		t.Fatalf("missing invalid-selection message:\n%s", out)
	}
	if !strings.Contains(out.String(), "must match vMAJOR.MINOR.PATCH") {
		t.Fatalf("missing invalid-version message:\n%s", out)
	}
}

func TestSelectVersionCancelAndEOF(t *testing.T) {
	for _, input := range []string{"5\n", ""} {
		t.Run(input, func(t *testing.T) {
			p := NewPrompter(strings.NewReader(input), io.Discard)
			_, err := p.SelectVersion(Version{}, nil)
			if !errors.Is(err, ErrCancelled) {
				t.Fatalf("SelectVersion() error = %v, want ErrCancelled", err)
			}
		})
	}
}

func TestConfirmRequiresExplicitYes(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "y\n", want: true},
		{input: "YES\n", want: true},
		{input: "\n", want: false},
		{input: "no\n", want: false},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			p := NewPrompter(strings.NewReader(test.input), io.Discard)
			got, err := p.Confirm()
			if err != nil || got != test.want {
				t.Fatalf("Confirm() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestConfirmEOFCancels(t *testing.T) {
	p := NewPrompter(strings.NewReader(""), io.Discard)
	_, err := p.Confirm()
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Confirm() error = %v, want ErrCancelled", err)
	}
}
