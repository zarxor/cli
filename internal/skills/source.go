package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxSkillArchiveSize = 64 << 20

type sourceKind string

const (
	sourceLocal   sourceKind = "local"
	sourceGitHub  sourceKind = "github"
	sourceArchive sourceKind = "archive"
)

type source struct {
	Raw     string
	Kind    sourceKind
	Local   string
	URL     string
	Owner   string
	Repo    string
	Subpath string
	Ref     string
}

func parseSource(raw, baseDir string) (source, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return source{}, fmt.Errorf("skill source cannot be empty")
	}

	if strings.HasPrefix(raw, "github:") {
		return parseGitHubSource(raw, strings.TrimPrefix(raw, "github:"))
	}
	if strings.HasPrefix(raw, "file:") {
		raw = strings.TrimPrefix(raw, "file:")
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, ".") || strings.HasPrefix(raw, "/") {
		return localSource(raw, baseDir)
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" {
		if parsed.Host == "github.com" {
			if github, ok := parseGitHubURL(parsed); ok {
				return github, nil
			}
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return source{}, fmt.Errorf("unsupported skill source scheme %q", parsed.Scheme)
		}
		if !strings.HasSuffix(strings.ToLower(parsed.Path), ".zip") {
			return source{}, fmt.Errorf("URL skill sources must be GitHub directories or .zip archives")
		}
		return source{Raw: raw, Kind: sourceArchive, URL: raw}, nil
	}

	return localSource(raw, baseDir)
}

func localSource(raw, baseDir string) (source, error) {
	local := raw
	if !filepath.IsAbs(local) {
		local = filepath.Join(baseDir, local)
	}
	local, err := filepath.Abs(local)
	if err != nil {
		return source{}, fmt.Errorf("resolve skill source %q: %w", raw, err)
	}
	return source{Raw: raw, Kind: sourceLocal, Local: local}, nil
}

func parseGitHubSource(raw, value string) (source, error) {
	ref := "main"
	if before, after, ok := strings.Cut(value, "@"); ok {
		value = before
		ref = strings.TrimSpace(after)
		if ref == "" {
			return source{}, fmt.Errorf("GitHub skill source ref cannot be empty")
		}
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return source{}, fmt.Errorf("invalid GitHub skill source %q (want github:owner/repo/path@ref)", raw)
	}
	return source{
		Raw:     raw,
		Kind:    sourceGitHub,
		Owner:   parts[0],
		Repo:    parts[1],
		Subpath: filepath.Join(parts[2:]...),
		Ref:     ref,
	}, nil
}

func parseGitHubURL(parsed *url.URL) (source, bool) {
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return source{}, false
	}
	if len(parts) >= 4 && parts[2] == "tree" && parts[3] != "" {
		return source{
			Raw:     parsed.String(),
			Kind:    sourceGitHub,
			Owner:   parts[0],
			Repo:    parts[1],
			Ref:     parts[3],
			Subpath: filepath.Join(parts[4:]...),
		}, true
	}
	if len(parts) == 2 {
		return source{
			Raw:   parsed.String(),
			Kind:  sourceGitHub,
			Owner: parts[0],
			Repo:  parts[1],
			Ref:   "main",
		}, true
	}
	return source{}, false
}

func (s source) archiveURL() string {
	if s.Kind == sourceArchive {
		return s.URL
	}
	return "https://codeload.github.com/" + url.PathEscape(s.Owner) + "/" + url.PathEscape(s.Repo) + "/zip/" + url.PathEscape(s.Ref)
}

func (m *Manager) loadSource(ctx context.Context, raw string) (source, string, func(), error) {
	sourceValue, err := parseSource(raw, m.env.WorkDir)
	if err != nil {
		return source{}, "", func() {}, err
	}

	switch sourceValue.Kind {
	case sourceLocal:
		root, err := localSkillRoot(sourceValue.Local)
		return sourceValue, root, func() {}, err
	case sourceGitHub, sourceArchive:
		archive, err := m.download(ctx, sourceValue.archiveURL())
		if err != nil {
			return source{}, "", func() {}, err
		}
		root, cleanup, err := extractSkillArchive(archive, sourceValue.Subpath)
		if err != nil {
			return source{}, "", func() {}, err
		}
		return sourceValue, root, cleanup, nil
	default:
		return source{}, "", func() {}, fmt.Errorf("unsupported skill source kind %q", sourceValue.Kind)
	}
}

func localSkillRoot(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read local skill source %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local skill source %q is not a directory", path)
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		return "", fmt.Errorf("local skill source %q does not contain SKILL.md", path)
	}
	return path, nil
}

func (m *Manager) download(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("create skill download request: %w", err)
	}
	request.Header.Set("User-Agent", "jb-agent-skills")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download skill source: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download skill source: HTTP %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSkillArchiveSize+1))
	if err != nil {
		return nil, fmt.Errorf("read skill archive: %w", err)
	}
	if len(data) > maxSkillArchiveSize {
		return nil, fmt.Errorf("skill archive exceeds %d MiB", maxSkillArchiveSize/(1<<20))
	}
	return data, nil
}

func extractSkillArchive(data []byte, subpath string) (string, func(), error) {
	base, cleanup, err := extractSkillArchiveRoot(data)
	if err != nil {
		return "", func() {}, err
	}
	root := base
	if subpath != "" {
		root = filepath.Join(base, filepath.FromSlash(subpath))
	}
	root, err = locateSkillRoot(root, subpath == "")
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return root, cleanup, nil
}

func extractSkillArchiveRoot(data []byte) (string, func(), error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", func() {}, fmt.Errorf("read skill archive: %w", err)
	}
	temporary, err := os.MkdirTemp("", "jb-skill-source-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create skill staging directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	for _, entry := range archive.File {
		if err := extractZipEntry(temporary, entry); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}

	base := archiveBase(temporary)
	return base, cleanup, nil
}

func extractZipEntry(base string, entry *zip.File) error {
	name := filepath.FromSlash(entry.Name)
	if name == "." || name == "" || filepath.IsAbs(name) {
		return fmt.Errorf("unsafe skill archive entry %q", entry.Name)
	}
	destination := filepath.Join(base, name)
	relative, err := filepath.Rel(base, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe skill archive entry %q", entry.Name)
	}
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(destination, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create skill archive directory: %w", err)
	}
	reader, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open skill archive entry %q: %w", entry.Name, err)
	}
	defer reader.Close()
	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create skill archive entry %q: %w", entry.Name, err)
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("extract skill archive entry %q: %w", entry.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close skill archive entry %q: %w", entry.Name, closeErr)
	}
	return nil
}

func archiveBase(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return root
	}
	return filepath.Join(root, entries[0].Name())
}

func locateSkillRoot(root string, discoverSingleChild bool) (string, error) {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return root, nil
	}
	if !discoverSingleChild {
		return "", fmt.Errorf("skill source path %q does not contain SKILL.md", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("inspect skill archive: %w", err)
	}
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
			candidates = append(candidates, filepath.Join(root, entry.Name()))
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("skill archive does not contain a directory with SKILL.md")
	}
	return "", fmt.Errorf("skill archive contains multiple skills; include the skill path in the source")
}
