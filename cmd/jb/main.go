package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/zarxor/scripts/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:]); err != nil {
		reportError(os.Stderr, err)
		os.Exit(1)
	}
}

func reportError(writer io.Writer, err error) {
	if renderErr := cli.PrintError(writer, err); renderErr != nil {
		fmt.Fprintln(writer, err)
	}
}
