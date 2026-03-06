package lean

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type IndexSnapshot struct {
	Root          string
	Files         map[string]FileRecord
	SymbolsBy     map[string][]SymbolRef
	Dependents    map[string][]string
	TestsBySrc    map[string][]string
	SourceByTest  map[string]string
	WorkspaceHash string
	UpdatedAt     time.Time
}

type Indexer struct {
	root string

	mu            sync.RWMutex
	files         map[string]FileRecord
	symbolsBy     map[string][]SymbolRef
	dependents    map[string][]string
	testsBySrc    map[string][]string
	sourceByTest  map[string]string
	workspaceHash string
	updatedAt     time.Time

	watcher     *fsnotify.Watcher
	watchCancel context.CancelFunc
}

var skipDirs = map[string]struct{}{
	".git":               {},
	".svn":               {},
	".hg":                {},
	"node_modules":       {},
	"vendor":             {},
	"dist":               {},
	"build":              {},
	"target":             {},
	"bin":                {},
	"obj":                {},
	".idea":              {},
	".vscode":            {},
	".lean-context-mode": {},
}

var extensionLanguage = map[string]string{
	".go":         "go",
	".ts":         "typescript",
	".tsx":        "typescript",
	".js":         "javascript",
	".jsx":        "javascript",
	".mjs":        "javascript",
	".cjs":        "javascript",
	".py":         "python",
	".java":       "java",
	".rs":         "rust",
	".cs":         "csharp",
	".tf":         "terraform",
	".tfvars":     "terraform",
	".yaml":       "yaml",
	".yml":        "yaml",
	".json":       "json",
	".xml":        "xml",
	".toml":       "toml",
	".gradle":     "gradle",
	".properties": "properties",
	".md":         "markdown",
	".sh":         "shell",
}

var importPatterns = map[string]*regexp.Regexp{
	"go":         regexp.MustCompile(`(?m)^\s*import\s+(?:\(|\"([^\"]+)\"|[A-Za-z_][\w]*\s+\"([^\"]+)\")`),
	"typescript": regexp.MustCompile(`(?m)^\s*import\s+(?:.+?from\s+)?["']([^"']+)["']`),
	"javascript": regexp.MustCompile(`(?m)^\s*import\s+(?:.+?from\s+)?["']([^"']+)["']`),
	"python":     regexp.MustCompile(`(?m)^\s*(?:from\s+([\w\.]+)\s+import|import\s+([\w\.]+))`),
	"java":       regexp.MustCompile(`(?m)^\s*import\s+([\w\.\*]+);`),
	"csharp":     regexp.MustCompile(`(?m)^\s*using\s+([\w\.]+);`),
	"rust":       regexp.MustCompile(`(?m)^\s*use\s+([\w:]+)`),
	"terraform":  regexp.MustCompile(`(?m)^\s*(module|resource|data|provider|variable|output)\s+"([^"]+)"`),
}

var symbolPatterns = map[string][]struct {
	kind string
	re   *regexp.Regexp
}{
	"go": {
		{kind: "method", re: regexp.MustCompile(`^\s*func\s*\([^)]*\)\s*([A-Za-z_][\w]*)\s*\(`)},
		{kind: "function", re: regexp.MustCompile(`^\s*func\s+([A-Za-z_][\w]*)\s*\(`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*type\s+([A-Za-z_][\w]*)\s+struct\b`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*type\s+([A-Za-z_][\w]*)\s+interface\b`)},
	},
	"typescript": {
		{kind: "function", re: regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)},
		{kind: "const", re: regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`)},
		{kind: "class", re: regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_$][\w$]*)\b`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][\w$]*)\b`)},
		{kind: "type", re: regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][\w$]*)\s*=`)},
	},
	"javascript": {
		{kind: "function", re: regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)},
		{kind: "const", re: regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`)},
		{kind: "class", re: regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_$][\w$]*)\b`)},
	},
	"python": {
		{kind: "function", re: regexp.MustCompile(`^\s*def\s+([A-Za-z_][\w]*)\s*\(`)},
		{kind: "class", re: regexp.MustCompile(`^\s*class\s+([A-Za-z_][\w]*)\s*(?:\(|:)`)},
	},
	"java": {
		{kind: "class", re: regexp.MustCompile(`^\s*(?:public\s+)?class\s+([A-Za-z_][\w]*)\b`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*(?:public\s+)?interface\s+([A-Za-z_][\w]*)\b`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:public\s+)?enum\s+([A-Za-z_][\w]*)\b`)},
		{kind: "method", re: regexp.MustCompile(`^\s*(?:public|private|protected)?\s*(?:static\s+)?[A-Za-z_<>,\[\]\?]+\s+([A-Za-z_][\w]*)\s*\(`)},
	},
	"csharp": {
		{kind: "class", re: regexp.MustCompile(`^\s*(?:public|internal|private|protected)?\s*(?:sealed\s+|abstract\s+)?class\s+([A-Za-z_][\w]*)`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*(?:public|internal|private|protected)?\s*interface\s+([A-Za-z_][\w]*)`)},
		{kind: "method", re: regexp.MustCompile(`^\s*(?:public|internal|private|protected)\s+(?:async\s+)?[A-Za-z_<>,\[\]\?]+\s+([A-Za-z_][\w]*)\s*\(`)},
	},
	"rust": {
		{kind: "function", re: regexp.MustCompile(`^\s*(?:pub\s+)?fn\s+([A-Za-z_][\w]*)\s*\(`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*(?:pub\s+)?struct\s+([A-Za-z_][\w]*)\b`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:pub\s+)?enum\s+([A-Za-z_][\w]*)\b`)},
		{kind: "trait", re: regexp.MustCompile(`^\s*(?:pub\s+)?trait\s+([A-Za-z_][\w]*)\b`)},
	},
	"terraform": {
		{kind: "resource", re: regexp.MustCompile(`^\s*resource\s+"([^"]+)"\s+"([^"]+)"`)},
		{kind: "module", re: regexp.MustCompile(`^\s*module\s+"([^"]+)"`)},
		{kind: "variable", re: regexp.MustCompile(`^\s*variable\s+"([^"]+)"`)},
		{kind: "output", re: regexp.MustCompile(`^\s*output\s+"([^"]+)"`)},
	},
	"yaml": {
		{kind: "kind", re: regexp.MustCompile(`^\s*kind:\s*([A-Za-z0-9_-]+)`)},
		{kind: "name", re: regexp.MustCompile(`^\s*name:\s*([A-Za-z0-9_.-]+)`)},
	},
}

var callPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

func NewIndexer(root string) (*Indexer, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Indexer{
		root:         cleanRoot,
		files:        map[string]FileRecord{},
		symbolsBy:    map[string][]SymbolRef{},
		dependents:   map[string][]string{},
		testsBySrc:   map[string][]string{},
		sourceByTest: map[string]string{},
	}, nil
}

func (ix *Indexer) Root() string { return ix.root }

func (ix *Indexer) Build(ctx context.Context) error {
	newFiles := map[string]FileRecord{}
	if err := filepath.WalkDir(ix.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		name := d.Name()
		if d.IsDir() {
			if _, skip := skipDirs[name]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		lang, ok := extensionLanguage[ext]
		if !ok {
			if name == "Dockerfile" || strings.HasSuffix(name, ".csproj") || name == "go.mod" || name == "package.json" || name == "pom.xml" || name == "Cargo.toml" {
				lang = "config"
			} else {
				return nil
			}
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.Size() > 2*1024*1024 {
			return nil
		}
		rec, parseErr := ix.parseFile(path, lang, info.Size())
		if parseErr != nil {
			return nil
		}
		newFiles[rec.Path] = rec
		return nil
	}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	newSymbols := map[string][]SymbolRef{}
	newDependents := map[string][]string{}
	newTestsBySrc := map[string][]string{}
	newSourceByTest := map[string]string{}
	workspaceHasher := sha256.New()

	for rel, rec := range newFiles {
		io.WriteString(workspaceHasher, rel+"|"+rec.Hash+"\n")
		for _, sym := range rec.Symbols {
			key := strings.ToLower(sym.Name)
			newSymbols[key] = append(newSymbols[key], sym)
		}
		for _, imp := range rec.Imports {
			for depRel := range newFiles {
				if depRel == rel {
					continue
				}
				depBase := strings.TrimSuffix(filepath.Base(depRel), filepath.Ext(depRel))
				if strings.Contains(imp, depBase) {
					newDependents[depRel] = appendUnique(newDependents[depRel], rel)
				}
			}
		}
		if rec.IsTest {
			src := sourceFromTest(rel)
			if src != "" {
				newTestsBySrc[src] = appendUnique(newTestsBySrc[src], rel)
				newSourceByTest[rel] = src
			}
		}
	}

	for k, v := range newSymbols {
		sort.Slice(v, func(i, j int) bool {
			if v[i].Score != v[j].Score {
				return v[i].Score > v[j].Score
			}
			if v[i].File != v[j].File {
				return v[i].File < v[j].File
			}
			return v[i].LineStart < v[j].LineStart
		})
		newSymbols[k] = v
	}

	ix.mu.Lock()
	ix.files = newFiles
	ix.symbolsBy = newSymbols
	ix.dependents = newDependents
	ix.testsBySrc = newTestsBySrc
	ix.sourceByTest = newSourceByTest
	ix.workspaceHash = hex.EncodeToString(workspaceHasher.Sum(nil))[:16]
	ix.updatedAt = time.Now().UTC()
	ix.mu.Unlock()
	return nil
}

func (ix *Indexer) StartWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := filepath.WalkDir(ix.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if _, skip := skipDirs[d.Name()]; skip {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	}); err != nil {
		watcher.Close()
		return err
	}

	innerCtx, cancel := context.WithCancel(ctx)
	ix.mu.Lock()
	ix.watcher = watcher
	ix.watchCancel = cancel
	ix.mu.Unlock()

	go func() {
		defer watcher.Close()
		debounce := time.NewTimer(time.Hour)
		if !debounce.Stop() {
			<-debounce.C
		}
		dirty := false
		for {
			select {
			case <-innerCtx.Done():
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
					dirty = true
					if ev.Op&fsnotify.Create != 0 {
						if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
							_ = watcher.Add(ev.Name)
						}
					}
					if !debounce.Stop() {
						select {
						case <-debounce.C:
						default:
						}
					}
					debounce.Reset(350 * time.Millisecond)
				}
			case <-debounce.C:
				if dirty {
					_ = ix.Build(innerCtx)
					dirty = false
				}
			case <-watcher.Errors:
				// ignore noisy watcher errors; full rebuild on next event is enough.
			}
		}
	}()

	return nil
}

func (ix *Indexer) StopWatcher() {
	ix.mu.Lock()
	cancel := ix.watchCancel
	ix.watchCancel = nil
	ix.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (ix *Indexer) Snapshot() IndexSnapshot {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	files := make(map[string]FileRecord, len(ix.files))
	for k, v := range ix.files {
		files[k] = v
	}
	symbolsBy := make(map[string][]SymbolRef, len(ix.symbolsBy))
	for k, v := range ix.symbolsBy {
		cp := make([]SymbolRef, len(v))
		copy(cp, v)
		symbolsBy[k] = cp
	}
	dependents := copyStrSliceMap(ix.dependents)
	testsBySrc := copyStrSliceMap(ix.testsBySrc)
	sourceByTest := make(map[string]string, len(ix.sourceByTest))
	for k, v := range ix.sourceByTest {
		sourceByTest[k] = v
	}
	return IndexSnapshot{
		Root:          ix.root,
		Files:         files,
		SymbolsBy:     symbolsBy,
		Dependents:    dependents,
		TestsBySrc:    testsBySrc,
		SourceByTest:  sourceByTest,
		WorkspaceHash: ix.workspaceHash,
		UpdatedAt:     ix.updatedAt,
	}
}

func (ix *Indexer) parseFile(absPath, language string, size int64) (FileRecord, error) {
	_, rel, err := NormalizeWorkspacePath(ix.root, absPath)
	if err != nil {
		return FileRecord{}, err
	}
	f, err := os.Open(absPath)
	if err != nil {
		return FileRecord{}, err
	}
	defer f.Close()

	h := sha256.New()
	tee := io.TeeReader(f, h)
	buf, err := io.ReadAll(tee)
	if err != nil {
		return FileRecord{}, err
	}
	content := string(buf)
	lines := splitLines(content)
	imports := extractImports(language, content)
	symbols := extractSymbols(language, rel, lines)
	calls := extractCalls(lines)
	isConfig := isConfigFile(rel)
	isTest := isTestFile(rel)
	tags := detectHeuristics(rel, lines)

	return FileRecord{
		Path:      rel,
		Language:  language,
		Hash:      hex.EncodeToString(h.Sum(nil))[:16],
		Size:      size,
		Imports:   imports,
		Calls:     calls,
		Symbols:   symbols,
		IsConfig:  isConfig,
		IsTest:    isTest,
		Tags:      tags,
		UpdatedAt: time.Now().UTC(),
		LineCount: len(lines),
	}, nil
}

func splitLines(content string) []string {
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(norm, "\n")
}

func extractImports(language, content string) []string {
	pat, ok := importPatterns[language]
	if !ok {
		return nil
	}
	matches := pat.FindAllStringSubmatch(content, -1)
	imports := make([]string, 0, len(matches))
	for _, m := range matches {
		for i := 1; i < len(m); i++ {
			if strings.TrimSpace(m[i]) != "" {
				imports = appendUnique(imports, strings.TrimSpace(m[i]))
				break
			}
		}
	}
	return imports
}

func extractSymbols(language, rel string, lines []string) []SymbolRef {
	patterns, ok := symbolPatterns[language]
	if !ok {
		return nil
	}
	var out []SymbolRef
	for i, line := range lines {
		for _, p := range patterns {
			m := p.re.FindStringSubmatch(line)
			if len(m) == 0 {
				continue
			}
			name := strings.TrimSpace(m[len(m)-1])
			if language == "terraform" && len(m) > 2 {
				name = strings.TrimSpace(m[1] + "." + m[2])
			}
			if name == "" {
				continue
			}
			out = append(out, SymbolRef{
				Name:      name,
				Kind:      p.kind,
				Signature: strings.TrimSpace(line),
				File:      rel,
				LineStart: i + 1,
				LineEnd:   i + 1,
				Language:  language,
			})
			break
		}
	}
	if len(out) == 0 {
		return out
	}
	for i := 0; i < len(out); i++ {
		end := len(lines)
		if i+1 < len(out) {
			end = out[i+1].LineStart - 1
		}
		if end < out[i].LineStart {
			end = out[i].LineStart
		}
		out[i].LineEnd = end
	}
	return out
}

func extractCalls(lines []string) []string {
	seen := map[string]struct{}{}
	calls := []string{}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") {
			continue
		}
		for _, m := range callPattern.FindAllStringSubmatch(line, -1) {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			if len(name) < 2 {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func detectHeuristics(path string, lines []string) []string {
	joined := strings.ToLower(strings.Join(lines, "\n"))
	p := strings.ToLower(path)
	var tags []string
	if strings.HasSuffix(p, ".sln") || strings.HasSuffix(p, ".csproj") {
		tags = append(tags, "dotnet-solution")
	}
	if strings.Contains(joined, "addscoped(") || strings.Contains(joined, "addtransient(") || strings.Contains(joined, "addsingleton(") {
		tags = append(tags, "dotnet-di")
	}
	if strings.Contains(joined, "dbcontext") || strings.Contains(joined, "entityframework") || strings.Contains(joined, ".include(") {
		tags = append(tags, "dotnet-ef")
	}
	if strings.Contains(joined, "useeffect(") || strings.Contains(joined, "usestate(") || strings.Contains(joined, "usememo(") {
		tags = append(tags, "react-hooks")
	}
	if strings.Contains(joined, "react-router") || strings.Contains(joined, "createrouter") || strings.Contains(joined, "routes") {
		tags = append(tags, "react-routing")
	}
	if strings.Contains(joined, "axios") || strings.Contains(joined, "fetch(") {
		tags = append(tags, "api-client")
	}
	if strings.Contains(joined, "kind:") && strings.Contains(joined, "apiVersion:") {
		tags = append(tags, "kubernetes")
	}
	return uniqueStrings(tags)
}

func sourceFromTest(path string) string {
	s := path
	replacements := []string{"_test.go", ".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx", ".test.js", ".spec.js", "Tests.cs", "Test.cs"}
	for _, r := range replacements {
		if strings.HasSuffix(s, r) {
			return strings.TrimSuffix(s, r) + sourceSuffix(r)
		}
	}
	return ""
}

func sourceSuffix(replaced string) string {
	switch replaced {
	case "_test.go":
		return ".go"
	case ".test.ts", ".spec.ts":
		return ".ts"
	case ".test.tsx", ".spec.tsx":
		return ".tsx"
	case ".test.js", ".spec.js":
		return ".js"
	case "Tests.cs", "Test.cs":
		return ".cs"
	default:
		return ""
	}
}

func isConfigFile(rel string) bool {
	base := filepath.Base(rel)
	if base == "go.mod" || base == "go.sum" || base == "package.json" || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock" {
		return true
	}
	if base == "Cargo.toml" || base == "pom.xml" || base == "build.gradle" || base == "settings.gradle" || base == "Dockerfile" {
		return true
	}
	if strings.HasSuffix(base, ".csproj") || strings.HasSuffix(base, ".sln") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	return ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".ini" || ext == ".json" || ext == ".tfvars"
}

func isTestFile(rel string) bool {
	b := strings.ToLower(filepath.Base(rel))
	return strings.HasSuffix(b, "_test.go") || strings.Contains(b, ".test.") || strings.Contains(b, ".spec.") || strings.HasSuffix(b, "tests.cs") || strings.HasSuffix(b, "test.cs")
}

func (ix *Indexer) ReadLines(rel string, start, end int) ([]string, error) {
	ix.mu.RLock()
	_, ok := ix.files[rel]
	ix.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("file not indexed: %s", rel)
	}
	abs := filepath.Join(ix.root, filepath.FromSlash(rel))
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if end-start > 400 {
		end = start + 400
	}

	lines := []string{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo < start {
			continue
		}
		if lineNo > end {
			break
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func (ix *Indexer) RepoMap(maxDirs int) RepoMap {
	snap := ix.Snapshot()
	if maxDirs <= 0 {
		maxDirs = 12
	}
	langs := map[string]int{}
	configFiles := 0
	testFiles := 0
	symbolCount := 0
	dirs := map[string]int{}
	signals := map[string]int{}
	for _, f := range snap.Files {
		langs[f.Language]++
		if f.IsConfig {
			configFiles++
		}
		if f.IsTest {
			testFiles++
		}
		symbolCount += len(f.Symbols)
		d := topDir(f.Path)
		dirs[d]++
		for _, tag := range f.Tags {
			signals[tag]++
		}
	}
	top := topNDirs(dirs, maxDirs)
	importRel := 0
	for _, deps := range snap.Dependents {
		importRel += len(deps)
	}
	return RepoMap{
		Root:                 snap.Root,
		FilesIndexed:         len(snap.Files),
		SymbolsIndexed:       symbolCount,
		Languages:            langs,
		ConfigFiles:          configFiles,
		TestFiles:            testFiles,
		WorkspaceHash:        snap.WorkspaceHash,
		IndexesUpdatedAt:     snap.UpdatedAt,
		TopDirectories:       top,
		HeuristicSignals:     signals,
		ImportRelationshipSz: importRel,
	}
}

func topDir(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "."
	}
	return parts[0]
}

func topNDirs(freq map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(freq))
	for k, v := range freq {
		arr = append(arr, kv{k: k, v: v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	if len(arr) > n {
		arr = arr[:n]
	}
	out := make([]string, 0, len(arr))
	for _, kv := range arr {
		out = append(out, fmt.Sprintf("%s(%d)", kv.k, kv.v))
	}
	return out
}

func copyStrSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func appendUnique(items []string, v string) []string {
	for _, item := range items {
		if item == v {
			return items
		}
	}
	return append(items, v)
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}
