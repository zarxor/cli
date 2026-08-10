package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// NewManager creates a manager using the supplied environment. Empty paths use
// the current user's normal platform locations.
func NewManager(environment Environment) (*Manager, error) {
	if environment.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("find user home: %w", err)
		}
		environment.HomeDir = home
	}
	if environment.WorkDir == "" {
		workDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("find working directory: %w", err)
		}
		environment.WorkDir = workDir
	}
	if environment.ConfigDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("find user config directory: %w", err)
		}
		environment.ConfigDir = configDir
	}
	if environment.CodexHome == "" {
		environment.CodexHome = os.Getenv("CODEX_HOME")
	}
	if environment.CodexHome == "" {
		environment.CodexHome = filepath.Join(environment.HomeDir, ".codex")
	}
	if environment.HTTPClient == nil {
		environment.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	normalized, err := normalizeCatalog(Catalog)
	if err != nil {
		return nil, err
	}
	return &Manager{env: environment, httpClient: environment.HTTPClient, catalog: normalized}, nil
}

// NewManagerWithCatalog creates a manager with an explicit available-skills
// catalog. It is useful for distributors and keeps catalog choices separate
// from filesystem and network behavior.
func NewManagerWithCatalog(environment Environment, catalog []CatalogEntry) (*Manager, error) {
	manager, err := NewManager(environment)
	if err != nil {
		return nil, err
	}
	manager.catalog, err = normalizeCatalog(catalog)
	if err != nil {
		return nil, err
	}
	return manager, nil
}

// Available returns the skills exposed to catalog-backed commands.
func (m *Manager) Available() []CatalogEntry {
	return cloneCatalog(m.catalog)
}

// Root returns the discovery directory for a target and scope.
func (m *Manager) Root(target Target, scope Scope) (string, error) {
	if target == TargetAll {
		return "", fmt.Errorf("target all must be expanded before resolving a path")
	}
	if scope != ScopeUser && scope != ScopeProject {
		return "", unknownScopeError(string(scope))
	}
	base := m.env.HomeDir
	if scope == ScopeProject {
		base = m.env.WorkDir
	}
	switch target {
	case TargetCodex:
		if scope == ScopeUser {
			return filepath.Join(m.env.CodexHome, "skills"), nil
		}
		return filepath.Join(base, ".agents", "skills"), nil
	case TargetAgents:
		return filepath.Join(base, ".agents", "skills"), nil
	case TargetClaude:
		return filepath.Join(base, ".claude", "skills"), nil
	case TargetCopilot:
		if scope == ScopeUser {
			return filepath.Join(base, ".copilot", "skills"), nil
		}
		return filepath.Join(base, ".github", "skills"), nil
	default:
		return "", unknownTargetError(string(target))
	}
}

// LockPath returns the manifest path used for a scope.
func (m *Manager) LockPath(scope Scope) string {
	if scope == ScopeProject {
		return filepath.Join(m.env.WorkDir, ".jb", "skills.lock")
	}
	return filepath.Join(m.env.ConfigDir, "jb", "skills.lock")
}

// Install downloads or reads a skill and installs it for one or more targets.
func (m *Manager) Install(ctx context.Context, sourceValue string, options InstallOptions) ([]Result, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Target == "" {
		options.Target = TargetCodex
	}
	source, sourceRoot, cleanup, err := m.loadSource(ctx, sourceValue)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return m.installLoadedSource(ctx, source, sourceRoot, options)
}

func (m *Manager) installLoadedSource(_ context.Context, source source, sourceRoot string, options InstallOptions) ([]Result, error) {
	metadata, err := readMetadata(sourceRoot)
	if err != nil {
		return nil, err
	}
	if options.ExpectedName != "" && metadata.Name != options.ExpectedName {
		return nil, fmt.Errorf("skill source declares skill %q, want %q", metadata.Name, options.ExpectedName)
	}
	digest, err := directoryDigest(sourceRoot)
	if err != nil {
		return nil, err
	}
	lock, err := m.loadLock(options.Scope)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(Targets(options.Target)))
	for _, target := range Targets(options.Target) {
		root, err := m.Root(target, options.Scope)
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(root, metadata.Name)
		if samePath(sourceRoot, destination) {
			return nil, fmt.Errorf("skill %q is already at %s", metadata.Name, destination)
		}
		if _, statErr := os.Stat(destination); statErr == nil && !options.Force {
			if lock.find(metadata.Name, target, options.Scope) != nil {
				return nil, fmt.Errorf("skill %q is already managed for %s/%s; use skills update", metadata.Name, target, options.Scope)
			}
			return nil, fmt.Errorf("skill destination %s already exists; use --force only after reviewing it", destination)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect skill destination %s: %w", destination, statErr)
		}
		result := Result{Name: metadata.Name, Description: metadata.Description, Target: target, Scope: options.Scope, Path: destination}
		if options.DryRun {
			result.Status = "dry-run"
			result.Message = source.Raw
			results = append(results, result)
			continue
		}
		if err := replaceDirectory(sourceRoot, destination); err != nil {
			return nil, fmt.Errorf("install skill %q: %w", metadata.Name, err)
		}
		lock.upsert(LockEntry{
			Name:        metadata.Name,
			Description: metadata.Description,
			Source:      source.Raw,
			Target:      target,
			Scope:       options.Scope,
			Digest:      digest,
		})
		if err := m.writeLock(options.Scope, lock); err != nil {
			return nil, err
		}
		result.Status = "installed"
		result.Message = source.Raw
		results = append(results, result)
	}
	return results, nil
}

// List returns installed skills, including unmanaged directories and invalid metadata.
func (m *Manager) List(options InspectOptions) ([]Info, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Target == "" {
		options.Target = TargetCodex
	}
	lock, err := m.loadLock(options.Scope)
	if err != nil {
		return nil, err
	}
	nameFilter := makeNameFilter(options.Names)
	var result []Info
	for _, target := range Targets(options.Target) {
		root, err := m.Root(target, options.Scope)
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list skills in %s: %w", root, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !nameFilter(entry.Name()) {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if _, skillFileErr := os.Stat(filepath.Join(path, "SKILL.md")); os.IsNotExist(skillFileErr) {
				// Target roots can contain agent-managed collections such as
				// .system. Only directories with their own SKILL.md are skills.
				continue
			}
			metadata, metadataErr := readMetadata(path)
			info := Info{Path: path, Target: target, Scope: options.Scope, Valid: metadataErr == nil}
			if metadataErr != nil {
				info.Name = entry.Name()
				info.Error = metadataErr.Error()
			} else {
				info.Metadata = metadata
				info.Digest, metadataErr = directoryDigest(path)
				if metadataErr != nil {
					info.Valid = false
					info.Error = metadataErr.Error()
				}
			}
			if lockEntry := lock.find(info.Name, target, options.Scope); lockEntry != nil {
				info.Managed = true
				info.Source = lockEntry.Source
				if info.Digest == "" {
					info.Digest = lockEntry.Digest
				}
			}
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Update refreshes managed skills and returns only skills whose source changed.
func (m *Manager) Update(ctx context.Context, options UpdateOptions) ([]Result, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Target == "" {
		options.Target = TargetCodex
	}
	lock, err := m.loadLock(options.Scope)
	if err != nil {
		return nil, err
	}
	nameFilter := makeNameFilter(options.Names)
	entries := make([]LockEntry, 0)
	for _, entry := range lock.Skills {
		if !containsTarget(Targets(options.Target), entry.Target) || entry.Scope != options.Scope || !nameFilter(entry.Name) {
			continue
		}
		entries = append(entries, entry)
	}
	var results []Result
	for _, entry := range entries {
		source, sourceRoot, cleanup, err := m.loadSource(ctx, entry.Source)
		if err != nil {
			return nil, fmt.Errorf("check skill %q: %w", entry.Name, err)
		}
		metadata, metadataErr := readMetadata(sourceRoot)
		digest, digestErr := directoryDigest(sourceRoot)
		cleanup()
		if metadataErr != nil {
			return nil, fmt.Errorf("check skill %q: %w", entry.Name, metadataErr)
		}
		if digestErr != nil {
			return nil, digestErr
		}
		if metadata.Name != entry.Name {
			return nil, fmt.Errorf("skill source %q changed its name from %q to %q", source.Raw, entry.Name, metadata.Name)
		}
		if digest == entry.Digest {
			continue
		}
		root, err := m.Root(entry.Target, entry.Scope)
		if err != nil {
			return nil, err
		}
		result := Result{Name: entry.Name, Description: metadata.Description, Target: entry.Target, Scope: entry.Scope, Path: filepath.Join(root, entry.Name), Message: entry.Source}
		if options.DryRun {
			result.Status = "available"
			results = append(results, result)
			continue
		}
		_, sourceRoot, cleanup, err = m.loadSource(ctx, entry.Source)
		if err != nil {
			return nil, fmt.Errorf("download skill %q: %w", entry.Name, err)
		}
		err = replaceDirectory(sourceRoot, result.Path)
		cleanup()
		if err != nil {
			return nil, fmt.Errorf("update skill %q: %w", entry.Name, err)
		}
		entry.Description = metadata.Description
		entry.Digest = digest
		lock.upsert(entry)
		if err := m.writeLock(options.Scope, lock); err != nil {
			return nil, err
		}
		result.Status = "updated"
		results = append(results, result)
	}
	return results, nil
}

// Remove deletes only skills recorded in the manifest.
func (m *Manager) Remove(_ context.Context, options RemoveOptions) ([]Result, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Target == "" {
		options.Target = TargetCodex
	}
	if len(options.Names) == 0 {
		return nil, fmt.Errorf("skill name is required")
	}
	lock, err := m.loadLock(options.Scope)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, name := range options.Names {
		for _, target := range Targets(options.Target) {
			entry := lock.find(name, target, options.Scope)
			if entry == nil {
				return nil, fmt.Errorf("skill %q is not managed for %s/%s", name, target, options.Scope)
			}
			root, err := m.Root(target, options.Scope)
			if err != nil {
				return nil, err
			}
			result := Result{Name: name, Target: target, Scope: options.Scope, Path: filepath.Join(root, name), Status: "removed"}
			if options.DryRun {
				result.Status = "dry-run"
				results = append(results, result)
				continue
			}
			if err := os.RemoveAll(result.Path); err != nil {
				return nil, fmt.Errorf("remove skill %q: %w", name, err)
			}
			lock.remove(name, target, options.Scope)
			if err := m.writeLock(options.Scope, lock); err != nil {
				return nil, err
			}
			results = append(results, result)
		}
	}
	return results, nil
}

// Verify checks metadata, managed provenance, and content integrity.
func (m *Manager) Verify(_ context.Context, options InspectOptions) ([]Result, error) {
	infos, err := m.List(options)
	if err != nil {
		return nil, err
	}
	lock, err := m.loadLock(options.Scope)
	if err != nil {
		return nil, err
	}
	var results []Result
	seen := make(map[string]bool)
	for _, info := range infos {
		key := string(info.Target) + ":" + info.Name
		seen[key] = true
		result := Result{Name: info.Name, Description: info.Description, Target: info.Target, Scope: info.Scope, Path: info.Path}
		switch {
		case !info.Valid:
			result.Status = "invalid"
			result.Message = info.Error
		case !info.Managed:
			result.Status = "unmanaged"
			result.Message = "not recorded in the jb lock manifest"
		default:
			entry := lock.find(info.Name, info.Target, info.Scope)
			if entry.Digest != info.Digest {
				result.Status = "modified"
				result.Message = "installed files differ from the recorded digest"
			} else {
				result.Status = "valid"
			}
		}
		results = append(results, result)
	}
	for _, entry := range lock.Skills {
		if entry.Scope != options.Scope || !containsTarget(Targets(options.Target), entry.Target) || !makeNameFilter(options.Names)(entry.Name) {
			continue
		}
		key := string(entry.Target) + ":" + entry.Name
		if seen[key] {
			continue
		}
		root, err := m.Root(entry.Target, entry.Scope)
		if err != nil {
			return nil, err
		}
		results = append(results, Result{Name: entry.Name, Target: entry.Target, Scope: entry.Scope, Path: filepath.Join(root, entry.Name), Status: "missing", Message: "recorded in the lock manifest but not installed"})
	}
	return results, nil
}

// Doctor reports target roots and verification findings without changing anything.
func (m *Manager) Doctor(ctx context.Context, options InspectOptions) ([]Result, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Target == "" {
		options.Target = TargetCodex
	}
	results := make([]Result, 0)
	for _, target := range Targets(options.Target) {
		root, err := m.Root(target, options.Scope)
		if err != nil {
			return nil, err
		}
		status := "ready"
		message := "skill directory is available"
		if _, err := os.Stat(root); os.IsNotExist(err) {
			status = "empty"
			message = "skill directory has not been created yet"
		} else if err != nil {
			status = "error"
			message = err.Error()
		}
		results = append(results, Result{Target: target, Scope: options.Scope, Path: root, Status: status, Message: message})
	}
	verified, err := m.Verify(ctx, options)
	if err != nil {
		return nil, err
	}
	return append(results, verified...), nil
}

func (m *Manager) loadLock(scope Scope) (Lockfile, error) {
	path := m.LockPath(scope)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Lockfile{Version: 1}, nil
	}
	if err != nil {
		return Lockfile{}, fmt.Errorf("read skill lock manifest: %w", err)
	}
	var lock Lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lockfile{}, fmt.Errorf("parse skill lock manifest: %w", err)
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	return lock, nil
}

func (m *Manager) writeLock(scope Scope, lock Lockfile) error {
	lock.Version = 1
	sort.Slice(lock.Skills, func(i, j int) bool {
		if lock.Skills[i].Target != lock.Skills[j].Target {
			return lock.Skills[i].Target < lock.Skills[j].Target
		}
		return lock.Skills[i].Name < lock.Skills[j].Name
	})
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skill lock manifest: %w", err)
	}
	path := m.LockPath(scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create skill lock directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".skills.lock-")
	if err != nil {
		return fmt.Errorf("create skill lock staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write skill lock manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close skill lock manifest: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace skill lock manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate skill lock manifest: %w", err)
	}
	return nil
}

func (lock Lockfile) find(name string, target Target, scope Scope) *LockEntry {
	for index := range lock.Skills {
		entry := &lock.Skills[index]
		if entry.Name == name && entry.Target == target && entry.Scope == scope {
			return entry
		}
	}
	return nil
}

func (lock *Lockfile) upsert(entry LockEntry) {
	if existing := lock.find(entry.Name, entry.Target, entry.Scope); existing != nil {
		*existing = entry
		return
	}
	lock.Skills = append(lock.Skills, entry)
}

func (lock *Lockfile) remove(name string, target Target, scope Scope) {
	filtered := lock.Skills[:0]
	for _, entry := range lock.Skills {
		if entry.Name == name && entry.Target == target && entry.Scope == scope {
			continue
		}
		filtered = append(filtered, entry)
	}
	lock.Skills = filtered
}

func makeNameFilter(names []string) func(string) bool {
	if len(names) == 0 {
		return func(string) bool { return true }
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	return func(name string) bool {
		_, ok := allowed[name]
		return ok
	}
}

func containsTarget(targets []Target, target Target) bool {
	for _, candidate := range targets {
		if candidate == target {
			return true
		}
	}
	return false
}
