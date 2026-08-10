package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func readMetadata(root string) (Metadata, error) {
	data, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return Metadata{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Metadata{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return Metadata{}, fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	values := make(map[string]string)
	for _, line := range lines[1:end] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "name" && key != "description" {
			continue
		}
		values[key] = yamlScalar(strings.TrimSpace(value))
	}
	metadata := Metadata{Name: values["name"], Description: values["description"]}
	if metadata.Name == "" {
		return Metadata{}, fmt.Errorf("SKILL.md frontmatter is missing name")
	}
	if len(metadata.Name) > 64 || !skillNamePattern.MatchString(metadata.Name) {
		return Metadata{}, fmt.Errorf("skill name %q must use lowercase letters, numbers, and hyphens", metadata.Name)
	}
	if metadata.Description == "" {
		return Metadata{}, fmt.Errorf("SKILL.md frontmatter is missing description")
	}
	if len(metadata.Description) > 1024 {
		return Metadata{}, fmt.Errorf("skill description exceeds 1024 characters")
	}
	return metadata, nil
}

func yamlScalar(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	return value
}

func directoryDigest(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains unsupported symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash skill files: %w", err)
	}
	sort.Strings(files)
	digest := sha256.New()
	for _, relative := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return "", fmt.Errorf("read skill file %q: %w", relative, err)
		}
		_, _ = io.WriteString(digest, relative)
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func replaceDirectory(sourceRoot, destinationRoot string) error {
	sourceAbs, err := filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}
	destinationAbs, err := filepath.Abs(destinationRoot)
	if err != nil {
		return err
	}
	if samePath(sourceAbs, destinationAbs) {
		return fmt.Errorf("skill source is already the installation directory")
	}
	if err := os.MkdirAll(filepath.Dir(destinationRoot), 0o755); err != nil {
		return fmt.Errorf("create skill installation directory: %w", err)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destinationRoot), ".jb-skill-install-")
	if err != nil {
		return fmt.Errorf("create skill staging directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := copyDirectory(sourceRoot, temporary); err != nil {
		return err
	}
	if err := os.RemoveAll(destinationRoot); err != nil {
		return fmt.Errorf("replace existing skill directory: %w", err)
	}
	if err := os.Rename(temporary, destinationRoot); err != nil {
		return fmt.Errorf("activate skill directory: %w", err)
	}
	return nil
}

func copyDirectory(sourceRoot, destinationRoot string) error {
	return filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains unsupported symbolic link %q", path)
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(destinationRoot, relative)
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			mode := fileInfo.Mode().Perm()
			if mode == 0 {
				mode = 0o755
			}
			return os.MkdirAll(destination, mode)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		mode := fileInfo.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return outputCloseErr
	})
}

func samePath(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}
