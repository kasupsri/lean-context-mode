package lean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type RootManager struct {
	ctx context.Context

	mu       sync.RWMutex
	active   string
	allowed  []string
	services map[string]*Service
}

func NewRootManager(ctx context.Context, initialRoot string) (*RootManager, error) {
	initialAbs, err := filepath.Abs(initialRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve initial root: %w", err)
	}
	if err := ensureDir(initialAbs); err != nil {
		return nil, err
	}
	allowed, err := resolveAllowedRoots(initialAbs)
	if err != nil {
		return nil, err
	}
	rm := &RootManager{
		ctx:      ctx,
		active:   initialAbs,
		allowed:  allowed,
		services: map[string]*Service{},
	}
	if _, err := rm.ensureService(initialAbs); err != nil {
		return nil, err
	}
	return rm, nil
}

func (rm *RootManager) CurrentRoot() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.active
}

func (rm *RootManager) AllowedRoots() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	cp := make([]string, len(rm.allowed))
	copy(cp, rm.allowed)
	return cp
}

func (rm *RootManager) ServiceFor(root string) (*Service, string, error) {
	resolved, err := rm.resolveRoot(root)
	if err != nil {
		return nil, "", err
	}
	svc, err := rm.ensureService(resolved)
	if err != nil {
		return nil, "", err
	}
	return svc, resolved, nil
}

func (rm *RootManager) SetActiveRoot(root string) (string, error) {
	resolved, err := rm.resolveRoot(root)
	if err != nil {
		return "", err
	}
	if _, err := rm.ensureService(resolved); err != nil {
		return "", err
	}
	rm.mu.Lock()
	rm.active = resolved
	rm.mu.Unlock()
	return resolved, nil
}

func (rm *RootManager) Stop() {
	rm.mu.Lock()
	services := make([]*Service, 0, len(rm.services))
	for _, svc := range rm.services {
		services = append(services, svc)
	}
	rm.services = map[string]*Service{}
	rm.mu.Unlock()
	for _, svc := range services {
		svc.Stop()
	}
}

func (rm *RootManager) resolveRoot(candidate string) (string, error) {
	rm.mu.RLock()
	active := rm.active
	allowed := make([]string, len(rm.allowed))
	copy(allowed, rm.allowed)
	rm.mu.RUnlock()

	if strings.TrimSpace(candidate) == "" {
		return active, nil
	}
	abs, err := filepath.Abs(strings.TrimSpace(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve workspace_root: %w", err)
	}
	if err := ensureDir(abs); err != nil {
		return "", err
	}
	if !isAllowedRoot(abs, allowed) {
		return "", fmt.Errorf("workspace_root %q is outside allowed roots", abs)
	}
	return abs, nil
}

func (rm *RootManager) ensureService(root string) (*Service, error) {
	rm.mu.RLock()
	existing := rm.services[root]
	rm.mu.RUnlock()
	if existing != nil {
		return existing, nil
	}

	svc, err := NewService(root)
	if err != nil {
		return nil, err
	}
	if err := svc.Start(rm.ctx); err != nil {
		return nil, err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()
	if already := rm.services[root]; already != nil {
		// Another goroutine won the race.
		svc.Stop()
		return already, nil
	}
	rm.services[root] = svc
	return svc, nil
}

func resolveAllowedRoots(initialAbs string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("LCM_ALLOWED_ROOTS"))
	roots := []string{}
	if raw != "" {
		for _, part := range splitRootList(raw) {
			abs, err := filepath.Abs(part)
			if err != nil {
				return nil, fmt.Errorf("resolve LCM_ALLOWED_ROOTS entry %q: %w", part, err)
			}
			if err := ensureDir(abs); err != nil {
				return nil, fmt.Errorf("LCM_ALLOWED_ROOTS entry invalid %q: %w", abs, err)
			}
			roots = append(roots, abs)
		}
	}
	if len(roots) == 0 {
		parent := filepath.Dir(initialAbs)
		if err := ensureDir(parent); err == nil {
			roots = append(roots, parent)
		}
	}
	roots = append(roots, initialAbs)
	return uniqueCleanPaths(roots), nil
}

func splitRootList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ','
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trim := strings.TrimSpace(p)
		if trim != "" {
			out = append(out, trim)
		}
	}
	return out
}

func uniqueCleanPaths(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		cp := filepath.Clean(p)
		if _, ok := seen[cp]; ok {
			continue
		}
		seen[cp] = struct{}{}
		out = append(out, cp)
	}
	return out
}

func isAllowedRoot(candidate string, allowed []string) bool {
	cand := filepath.Clean(candidate)
	for _, a := range allowed {
		base := filepath.Clean(a)
		rel, err := filepath.Rel(base, cand)
		if err != nil {
			continue
		}
		if rel == "." {
			return true
		}
		if !strings.HasPrefix(rel, "..") && rel != "" {
			return true
		}
	}
	return false
}

func ensureDir(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path %q unavailable: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	return nil
}
