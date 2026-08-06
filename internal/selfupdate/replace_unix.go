//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
)

func replaceExecutable(staged, destination string) (bool, error) {
	if err := os.Rename(staged, destination); err != nil {
		return false, fmt.Errorf("replace %s: %w", destination, err)
	}
	return false, nil
}
