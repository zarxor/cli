package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const catalogCheckWorkers = 4

type catalogSourceCache struct {
	mu       sync.Mutex
	archives map[string]string
	cleanups []func()
}

func newCatalogSourceCache() *catalogSourceCache {
	return &catalogSourceCache{archives: make(map[string]string)}
}

func (c *catalogSourceCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cleanup := range c.cleanups {
		cleanup()
	}
	c.cleanups = nil
	c.archives = nil
}

type catalogCheckResult struct {
	index  int
	status CatalogStatus
	err    error
}

func (m *Manager) loadCatalogSource(ctx context.Context, raw string, cache *catalogSourceCache) (source, string, func(), error) {
	if cache == nil {
		return m.loadSource(ctx, raw)
	}
	sourceValue, err := parseSource(raw, m.env.WorkDir)
	if err != nil {
		return source{}, "", func() {}, err
	}
	if sourceValue.Kind == sourceLocal {
		root, err := localSkillRoot(sourceValue.Local)
		return sourceValue, root, func() {}, err
	}

	cache.mu.Lock()
	root, ok := cache.archives[sourceValue.archiveURL()]
	if !ok {
		data, downloadErr := m.download(ctx, sourceValue.archiveURL())
		if downloadErr != nil {
			cache.mu.Unlock()
			return source{}, "", func() {}, downloadErr
		}
		extractedRoot, cleanup, extractErr := extractSkillArchiveRoot(data)
		if extractErr != nil {
			cache.mu.Unlock()
			return source{}, "", func() {}, extractErr
		}
		root = extractedRoot
		cache.archives[sourceValue.archiveURL()] = root
		cache.cleanups = append(cache.cleanups, cleanup)
	}
	cache.mu.Unlock()

	skillRoot := root
	if sourceValue.Subpath != "" {
		skillRoot = filepath.Join(root, filepath.FromSlash(sourceValue.Subpath))
	}
	skillRoot, err = locateSkillRoot(skillRoot, sourceValue.Subpath == "")
	if err != nil {
		return source{}, "", func() {}, err
	}
	return sourceValue, skillRoot, func() {}, nil
}

// CheckCatalog discovers local installation state for a catalog. When
// checkUpdates is true it also resolves each installed catalog source and
// compares its digest with the lock manifest.
func (m *Manager) CheckCatalog(ctx context.Context, entries []CatalogEntry, checkUpdates bool, progress CatalogProgress) ([]CatalogStatus, error) {
	normalized, err := normalizeCatalogEntries(entries, true)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}

	checkContext, cancel := context.WithCancel(ctx)
	defer cancel()
	sourceCache := newCatalogSourceCache()
	defer sourceCache.close()
	jobs := make(chan int)
	results := make(chan catalogCheckResult, len(normalized))
	workerCount := len(normalized)
	if workerCount > catalogCheckWorkers {
		workerCount = catalogCheckWorkers
	}

	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				status, checkErr := m.checkCatalogEntry(checkContext, normalized[index], checkUpdates, sourceCache)
				results <- catalogCheckResult{index: index, status: status, err: checkErr}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range normalized {
			select {
			case jobs <- index:
			case <-checkContext.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	statuses := make([]CatalogStatus, len(normalized))
	completed := 0
	var firstErr error
	for result := range results {
		completed++
		if result.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("check skill %q: %w", normalized[result.index].ID, result.err)
				cancel()
			}
			continue
		}
		statuses[result.index] = result.status
		if progress != nil && firstErr == nil {
			if progressErr := progress(completed, len(normalized)); progressErr != nil {
				firstErr = fmt.Errorf("render catalog progress: %w", progressErr)
				cancel()
			}
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return statuses, nil
}

func (m *Manager) checkCatalogEntry(ctx context.Context, entry CatalogEntry, checkUpdates bool, sourceCache *catalogSourceCache) (CatalogStatus, error) {
	status, err := m.inspectCatalogEntry(entry)
	if err != nil {
		return CatalogStatus{}, err
	}
	if !checkUpdates || !status.Installed || !status.Managed {
		return status, nil
	}
	if err := ctx.Err(); err != nil {
		return CatalogStatus{}, err
	}
	loaded, sourceRoot, cleanup, err := m.loadCatalogSource(ctx, entry.Source, sourceCache)
	if err != nil {
		return CatalogStatus{}, err
	}
	defer cleanup()
	metadata, err := readMetadata(sourceRoot)
	if err != nil {
		return CatalogStatus{}, err
	}
	if SkillID(metadata.Name) != entry.ID {
		return CatalogStatus{}, fmt.Errorf("catalog source declares skill %q, want %q", metadata.Name, entry.ID)
	}
	digest, err := directoryDigest(sourceRoot)
	if err != nil {
		return CatalogStatus{}, err
	}
	status.CandidateDigest = digest
	status.UpdateAvailable = digest != status.InstalledDigest || loaded.Raw != lockSource(status, m, entry)
	return status, nil
}

func (m *Manager) installCatalogEntry(ctx context.Context, entry CatalogEntry, options InstallOptions, sourceCache *catalogSourceCache) ([]Result, error) {
	loaded, sourceRoot, cleanup, err := m.loadCatalogSource(ctx, entry.Source, sourceCache)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return m.installLoadedSource(ctx, loaded, sourceRoot, options)
}

// inspectCatalogEntry only reads installed directories and the lock manifest.
func (m *Manager) inspectCatalogEntry(entry CatalogEntry) (CatalogStatus, error) {
	lock, err := m.loadLock(entry.Scope)
	if err != nil {
		return CatalogStatus{}, err
	}
	targets := Targets(entry.Target)
	status := CatalogStatus{Entry: entry}
	present := 0
	managed := 0
	for _, target := range targets {
		root, err := m.Root(target, entry.Scope)
		if err != nil {
			return CatalogStatus{}, err
		}
		destination := filepath.Join(root, string(entry.ID))
		if _, err := os.Stat(destination); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return CatalogStatus{}, fmt.Errorf("inspect skill destination %s: %w", destination, err)
		}
		present++
		metadata, metadataErr := readMetadata(destination)
		if metadataErr != nil {
			status.Message = metadataErr.Error()
			continue
		}
		if SkillID(metadata.Name) != entry.ID {
			status.Message = fmt.Sprintf("installed skill metadata is %q", metadata.Name)
			continue
		}
		digest, digestErr := directoryDigest(destination)
		if digestErr != nil {
			return CatalogStatus{}, digestErr
		}
		status.InstalledDigest = digest
		if lockEntry := lock.find(string(entry.ID), target, entry.Scope); lockEntry != nil {
			managed++
			if status.InstalledDigest == "" {
				status.InstalledDigest = lockEntry.Digest
			}
		}
	}
	status.Installed = present == len(targets)
	status.PartiallyInstalled = present > 0 && !status.Installed
	status.Managed = status.Installed && managed == len(targets)
	return status, nil
}

// InstallCatalog installs selected entries from the available catalog.
func (m *Manager) InstallCatalog(ctx context.Context, statuses []CatalogStatus, selected []SkillID, options CatalogOperationOptions) ([]Result, error) {
	selectedSet := make(map[SkillID]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	sourceCache := newCatalogSourceCache()
	defer sourceCache.close()
	var results []Result
	for _, status := range statuses {
		if _, ok := selectedSet[status.Entry.ID]; !ok {
			continue
		}
		if status.Installed || status.PartiallyInstalled {
			continue
		}
		if options.Progress != nil {
			if err := options.Progress("Installing " + status.Entry.Name + "…"); err != nil {
				return nil, err
			}
		}
		installed, err := m.installCatalogEntry(ctx, status.Entry, InstallOptions{
			Target:       status.Entry.Target,
			Scope:        status.Entry.Scope,
			DryRun:       options.DryRun,
			ExpectedName: string(status.Entry.ID),
		}, sourceCache)
		if err != nil {
			return nil, err
		}
		for _, result := range installed {
			result.Name = status.Entry.Name
			result.Message = ""
			if status.Entry.Name != "" {
				result.Description = status.Entry.Description
			}
			results = append(results, result)
		}
	}
	return results, nil
}

// UpdateCatalog updates selected entries whose catalog source has changed.
// The check phase determines eligibility; this phase performs the selected
// writes without accepting a source from the command line.
func (m *Manager) UpdateCatalog(ctx context.Context, statuses []CatalogStatus, selected []SkillID, options CatalogOperationOptions) ([]Result, error) {
	selectedSet := make(map[SkillID]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	sourceCache := newCatalogSourceCache()
	defer sourceCache.close()
	var results []Result
	for _, status := range statuses {
		if _, ok := selectedSet[status.Entry.ID]; !ok || !status.UpdateAvailable {
			continue
		}
		if options.Progress != nil {
			if err := options.Progress("Updating " + status.Entry.Name + "…"); err != nil {
				return nil, err
			}
		}
		updated, err := m.updateCatalogEntry(ctx, status.Entry, options.DryRun, sourceCache)
		if err != nil {
			return nil, err
		}
		for _, result := range updated {
			result.Message = ""
			if status.Entry.Name != "" {
				result.Description = status.Entry.Description
			}
			results = append(results, result)
		}
	}
	return results, nil
}

func (m *Manager) updateCatalogEntry(ctx context.Context, entry CatalogEntry, dryRun bool, sourceCache *catalogSourceCache) ([]Result, error) {
	loaded, sourceRoot, cleanup, err := m.loadCatalogSource(ctx, entry.Source, sourceCache)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	metadata, err := readMetadata(sourceRoot)
	if err != nil {
		return nil, err
	}
	if SkillID(metadata.Name) != entry.ID {
		return nil, fmt.Errorf("catalog source declares skill %q, want %q", metadata.Name, entry.ID)
	}
	digest, err := directoryDigest(sourceRoot)
	if err != nil {
		return nil, err
	}
	lock, err := m.loadLock(entry.Scope)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, target := range Targets(entry.Target) {
		root, err := m.Root(target, entry.Scope)
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(root, string(entry.ID))
		result := Result{Name: entry.Name, Description: metadata.Description, Target: target, Scope: entry.Scope, Path: destination}
		if dryRun {
			result.Status = "dry-run"
			results = append(results, result)
			continue
		}
		if err := replaceDirectory(sourceRoot, destination); err != nil {
			return nil, fmt.Errorf("update skill %q: %w", entry.ID, err)
		}
		lock.upsert(LockEntry{
			Name:        string(entry.ID),
			Description: metadata.Description,
			Source:      loaded.Raw,
			Target:      target,
			Scope:       entry.Scope,
			Digest:      digest,
		})
		if err := m.writeLock(entry.Scope, lock); err != nil {
			return nil, err
		}
		result.Status = "updated"
		results = append(results, result)
	}
	return results, nil
}

func lockSource(status CatalogStatus, manager *Manager, entry CatalogEntry) string {
	lock, err := manager.loadLock(entry.Scope)
	if err != nil {
		return ""
	}
	for _, target := range Targets(entry.Target) {
		if entry := lock.find(string(status.Entry.ID), target, status.Entry.Scope); entry != nil {
			return entry.Source
		}
	}
	return ""
}
