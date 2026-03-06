package lean

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type GitDiff struct {
	root string
}

func NewGitDiff(root string) *GitDiff {
	return &GitDiff{root: root}
}

func (g *GitDiff) status(ctx context.Context) ([]ChangedFile, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", g.root, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	files := make([]ChangedFile, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}
		xy := line[:2]
		path := strings.TrimSpace(line[3:])
		path = filepath.ToSlash(path)
		status := strings.TrimSpace(xy)
		f := ChangedFile{Path: path, Status: status, IsStaged: xy[0] != ' ' && xy[0] != '?', IsUnstaged: xy[1] != ' '}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

var hunkHeader = regexp.MustCompile(`^@@\s+\-(\d+)(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)
var changedSymbol = regexp.MustCompile(`(?m)^[\+\-]\s*(?:func|class|interface|type|enum|struct|trait|def|resource|module|variable|output)\s+([A-Za-z_][\w.-]*)`)

func (g *GitDiff) hunks(ctx context.Context, path string, maxHunks int, includeLines bool) ([]DiffHunk, error) {
	if maxHunks <= 0 {
		maxHunks = 3
	}
	cmd := exec.CommandContext(ctx, "git", "-C", g.root, "diff", "--unified=0", "--", path)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, nil
		}
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	hunks := []DiffHunk{}
	cur := DiffHunk{File: filepath.ToSlash(path), Lines: []string{}, Symbols: []string{}, IsUnstaged: true}
	collecting := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@") {
			if collecting {
				hunks = append(hunks, cur)
				if len(hunks) >= maxHunks {
					break
				}
				cur = DiffHunk{File: filepath.ToSlash(path), Lines: []string{}, Symbols: []string{}, IsUnstaged: true}
			}
			cur.Header = line
			collecting = true
			continue
		}
		if !collecting {
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			cur.Added++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			cur.Deleted++
		}
		if includeLines && len(cur.Lines) < 12 && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")) {
			cur.Lines = append(cur.Lines, line)
		}
		if m := changedSymbol.FindStringSubmatch(line); len(m) > 1 {
			cur.Symbols = appendUnique(cur.Symbols, m[1])
		}
	}
	if collecting && len(hunks) < maxHunks {
		hunks = append(hunks, cur)
	}
	return hunks, nil
}

func (g *GitDiff) Focus(ctx context.Context, in ChangesFocusInput, idx *Indexer) ChangesFocus {
	if in.MaxFiles <= 0 {
		in.MaxFiles = 40
	}
	if in.MaxHunksPerFile <= 0 {
		in.MaxHunksPerFile = 3
	}
	if !in.IncludeHunks {
		in.IncludeHunks = true
	}

	files, err := g.status(ctx)
	focus := ChangesFocus{Root: g.root, CollectedAt: time.Now().UTC()}
	if err != nil {
		focus.Warnings = append(focus.Warnings, "git status unavailable: "+err.Error())
		return focus
	}
	if len(files) > in.MaxFiles {
		files = files[:in.MaxFiles]
	}
	focus.Files = files

	affected := []SymbolRef{}
	hunks := []DiffHunk{}
	snap := idx.Snapshot()
	for _, file := range files {
		hs, hErr := g.hunks(ctx, file.Path, in.MaxHunksPerFile, in.IncludeHunks)
		if hErr != nil {
			focus.Warnings = append(focus.Warnings, "diff unavailable for "+file.Path+": "+hErr.Error())
			continue
		}
		for i := range hs {
			hs[i].IsStaged = file.IsStaged
			hs[i].IsUnstaged = file.IsUnstaged
		}
		hunks = append(hunks, hs...)
		if rec, ok := snap.Files[file.Path]; ok {
			for _, sym := range rec.Symbols {
				affected = append(affected, sym)
			}
		}
	}
	focus.Hunks = hunks
	if len(affected) > 200 {
		affected = affected[:200]
	}
	focus.Affected = affected
	return focus
}

func (g *GitDiff) ChangedFileSet(ctx context.Context) map[string]ChangedFile {
	files, err := g.status(ctx)
	if err != nil {
		return map[string]ChangedFile{}
	}
	m := make(map[string]ChangedFile, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}
