package gitstatus

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Status int

const (
	StatusClean Status = iota
	StatusModified
	StatusAdded
	StatusUntracked
	StatusDeleted
	StatusRenamed
	StatusConflict
)

type FileStatus struct {
	Path     string
	RepoPath string
	Status   Status
}

type Counts struct {
	Modified  int
	Added     int
	Untracked int
	Deleted   int
	Renamed   int
	Conflict  int
}

type Snapshot struct {
	RepoRoot string
	Branch   string
	Ahead    int
	Behind   int
	Files    map[string]FileStatus
}

var ErrGitUnavailable = errors.New("git unavailable")

func Load(ctx context.Context, workspaceRoot string, paths []string) (Snapshot, error) {
	repoRoot, err := findRepoRoot(ctx, workspaceRoot)
	if err != nil || repoRoot == "" {
		return Snapshot{}, err
	}

	repoPaths := repoRelativePaths(repoRoot, paths)
	if len(repoPaths) == 0 {
		return Snapshot{RepoRoot: repoRoot}, nil
	}

	args := []string{
		"-C", repoRoot,
		"status",
		"--porcelain=v2",
		"-z",
		"--branch",
		"--untracked-files=all",
		"--",
	}
	args = append(args, repoPaths...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Snapshot{}, ErrGitUnavailable
		}
		return Snapshot{}, fmt.Errorf("git status: %w", err)
	}
	return parsePorcelainV2(repoRoot, string(out)), nil
}

// An empty path with a nil error means root is not inside a Git repository.
func findRepoRoot(ctx context.Context, root string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitUnavailable
		}
		return "", nil
	}

	repoRoot := strings.TrimSpace(string(out))
	if repoRoot == "" {
		return "", nil
	}
	return canonicalPath(repoRoot), nil
}

func repoRelativePaths(repoRoot string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		abs := canonicalPath(path)
		rel, err := filepath.Rel(repoRoot, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func parsePorcelainV2(repoRoot string, out string) Snapshot {
	snap := Snapshot{
		RepoRoot: canonicalPath(repoRoot),
		Files:    make(map[string]FileStatus),
	}
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		if strings.HasPrefix(record, "# ") {
			parseBranchHeader(&snap, record)
			continue
		}

		status, repoPath, consumesNext := parseStatusRecord(record)
		if consumesNext {
			i++
		}
		if status == StatusClean || repoPath == "" {
			continue
		}
		snap.setFile(repoPath, status)
	}
	if len(snap.Files) == 0 {
		snap.Files = nil
	}
	return snap
}

func parseBranchHeader(snap *Snapshot, record string) {
	header := strings.TrimPrefix(record, "# ")
	key, value, ok := strings.Cut(header, " ")
	if !ok {
		return
	}

	switch key {
	case "branch.head":
		snap.Branch = value
	case "branch.ab":
		ahead, behind := parseAheadBehind(value)
		snap.Ahead = ahead
		snap.Behind = behind
	}
}

func parseAheadBehind(value string) (int, int) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, 0
	}
	ahead, _ := strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
	behind, _ := strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
	return ahead, behind
}

func parseStatusRecord(record string) (Status, string, bool) {
	switch record[0] {
	case '1':
		parts := strings.SplitN(record, " ", 9)
		if len(parts) != 9 {
			return StatusClean, "", false
		}
		return statusFromXY(parts[1]), parts[8], false
	case '2':
		parts := strings.SplitN(record, " ", 10)
		if len(parts) != 10 {
			return StatusClean, "", true
		}
		return StatusRenamed, parts[9], true
	case 'u':
		parts := strings.SplitN(record, " ", 11)
		if len(parts) != 11 {
			return StatusClean, "", false
		}
		return StatusConflict, parts[10], false
	case '?':
		return StatusUntracked, strings.TrimPrefix(record, "? "), false
	default:
		return StatusClean, "", false
	}
}

func statusFromXY(xy string) Status {
	if len(xy) != 2 {
		return StatusClean
	}

	switch {
	case strings.ContainsRune(xy, 'D'):
		return StatusDeleted
	case strings.ContainsRune(xy, 'A'):
		return StatusAdded
	case strings.ContainsAny(xy, "MTRC"):
		return StatusModified
	default:
		return StatusClean
	}
}

func (s *Snapshot) setFile(repoPath string, status Status) {
	path := canonicalPath(filepath.Join(s.RepoRoot, filepath.FromSlash(repoPath)))
	current, ok := s.Files[path]
	if ok && current.Status.priority() >= status.priority() {
		return
	}
	s.Files[path] = FileStatus{
		Path:     path,
		RepoPath: repoPath,
		Status:   status,
	}
}

func (s Snapshot) File(path string) (FileStatus, bool) {
	if len(s.Files) == 0 {
		return FileStatus{}, false
	}
	status, ok := s.Files[canonicalPath(path)]
	return status, ok
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		abs = filepath.Clean(path)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(real)
	}
	return filepath.Clean(abs)
}

func (s Snapshot) Counts() Counts {
	var counts Counts
	for _, file := range s.Files {
		switch file.Status {
		case StatusModified:
			counts.Modified++
		case StatusAdded:
			counts.Added++
		case StatusUntracked:
			counts.Untracked++
		case StatusDeleted:
			counts.Deleted++
		case StatusRenamed:
			counts.Renamed++
		case StatusConflict:
			counts.Conflict++
		}
	}
	return counts
}

func (c Counts) Any() bool {
	return c != (Counts{})
}

func (s Status) Label() string {
	switch s {
	case StatusModified:
		return "M"
	case StatusAdded:
		return "A"
	case StatusUntracked:
		return "U"
	case StatusDeleted:
		return "D"
	case StatusRenamed:
		return "R"
	case StatusConflict:
		return "!"
	default:
		return ""
	}
}

func (s Status) priority() int {
	switch s {
	case StatusConflict:
		return 6
	case StatusDeleted:
		return 5
	case StatusRenamed:
		return 4
	case StatusAdded:
		return 3
	case StatusModified:
		return 2
	case StatusUntracked:
		return 1
	default:
		return 0
	}
}
