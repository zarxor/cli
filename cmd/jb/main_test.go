package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestReportErrorFallsBackWhenSemanticRenderingFails(t *testing.T) {
	w := &failFirstWriter{err: errors.New("first write failed")}
	reportError(w, errors.New("boom"))
	if got, want := w.output.String(), "boom\n"; got != want {
		t.Fatalf("fallback output = %q, want %q", got, want)
	}
}

type failFirstWriter struct {
	writes int
	err    error
	output bytes.Buffer
}

func (w *failFirstWriter) Write(value []byte) (int, error) {
	if w.writes == 0 {
		w.writes++
		return 0, w.err
	}
	w.writes++
	return w.output.Write(value)
}
