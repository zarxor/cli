package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/zarxor/scripts/internal/release"
)

var errReleaseCancelled = release.ErrCancelled

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(context.Background(), dir, runtime.GOOS, os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, dir, hostOS string, in io.Reader, out, errOut io.Writer) int {
	err := release.Execute(ctx, release.Options{
		Dir:       dir,
		HostOS:    hostOS,
		In:        in,
		Out:       out,
		Runner:    release.ExecRunner{},
		MkTemp:    os.MkdirTemp,
		RemoveAll: os.RemoveAll,
	})
	if err != nil {
		if errors.Is(err, errReleaseCancelled) {
			fmt.Fprintln(out, "No release was published.")
		} else {
			fmt.Fprintln(errOut, err)
		}
	}
	return exitCodeForError(err)
}

func exitCodeForError(err error) int {
	if err == nil || errors.Is(err, errReleaseCancelled) {
		return 0
	}
	return 1
}
