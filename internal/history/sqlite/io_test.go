package sqlite

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcDB := filepath.Join(dir, "src.db")
	dstDB := filepath.Join(dir, "dst.db")
	out := filepath.Join(dir, "hist.json")

	src := New(srcDB)
	if err := src.Load(); err != nil {
		t.Fatalf("load src: %v", err)
	}

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	_ = src.Append(history.Entry{ID: "1", ExecutedAt: t1, Method: "GET", URL: "https://one.test"})
	_ = src.Append(history.Entry{ID: "2", ExecutedAt: t2, Method: "POST", URL: "https://two.test"})

	n, err := src.ExportJSON(out)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 exported rows, got %d", n)
	}

	dst := New(dstDB)
	if err := dst.Load(); err != nil {
		t.Fatalf("load dst: %v", err)
	}
	n, err = dst.ImportJSON(out)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 imported rows, got %d", n)
	}

	got, err := dst.Entries()
	if err != nil {
		t.Fatalf("dst entries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows in dst, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "1" {
		t.Fatalf("expected IDs 2,1 got %q,%q", got[0].ID, got[1].ID)
	}
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "hist.bak.db")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := s.Backup(out); err != nil {
		t.Fatalf("backup: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() <= 0 {
		t.Fatalf("expected non-empty backup file")
	}

	cpy := New(out)
	if err := cpy.Load(); err != nil {
		t.Fatalf("load backup db: %v", err)
	}
	got, err := cpy.Entries()
	if err != nil {
		t.Fatalf("backup entries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row in backup db, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Fatalf("expected backup row ID 1, got %q", got[0].ID)
	}
}

func TestBackupSamePathRejected(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Backup(db); err == nil {
		t.Fatalf("expected same-path backup error")
	}
}

func TestBackupQuotedPath(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "quoted'snapshot.db")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := s.Backup(out); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("stat backup: %v", err)
	}
}

func TestExportJSONKeepsTargetOnWriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod directory write bit semantics are not stable on windows")
	}

	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "out.json")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	const keep = "keep-this-content"
	if err := os.WriteFile(out, []byte(keep), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod readonly dir: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	if _, err := s.ExportJSON(out); err == nil {
		t.Fatalf("expected export error for readonly dir")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output after failed export: %v", err)
	}
	if string(got) != keep {
		t.Fatalf("expected original output content to stay intact")
	}
}

func TestExportJSONValidPayload(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "out.json")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = s.Append(
		history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET", URL: "https://one.test"},
	)

	if _, err := s.ExportJSON(out); err != nil {
		t.Fatalf("export: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	var es []history.Entry
	if err := json.Unmarshal(data, &es); err != nil {
		t.Fatalf("decode exported json: %v", err)
	}
	if len(es) != 1 || es[0].ID != "1" {
		t.Fatalf("unexpected exported rows: %+v", es)
	}
}

func TestBackupOverStaleWAL(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "b.db")
	old := New(dest)
	if err := old.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), URL: "old-backup"}); err != nil {
		t.Fatalf("append old: %v", err)
	}
	crashed := map[string][]byte{}
	for _, sfx := range []string{"", "-wal", "-shm"} {
		b, err := os.ReadFile(dest + sfx)
		if err != nil {
			t.Fatalf("read %s: %v", sfx, err)
		}
		crashed[sfx] = b
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old: %v", err)
	}
	for sfx, b := range crashed {
		if err := os.WriteFile(dest+sfx, b, 0o600); err != nil {
			t.Fatalf("restore %s: %v", sfx, err)
		}
	}

	src := New(filepath.Join(dir, "src.db"))
	defer func() { _ = src.Close() }()
	if err := src.Append(history.Entry{ID: "2", ExecutedAt: time.Now(), URL: "source"}); err != nil {
		t.Fatalf("append source: %v", err)
	}
	if err := src.Backup(dest); err != nil {
		t.Fatalf("backup: %v", err)
	}
	got := New(dest)
	defer func() { _ = got.Close() }()
	es, err := got.Entries()
	if err != nil || len(es) != 1 || es[0].URL != "source" {
		t.Fatalf("backup holds %+v, %v, want only source", es, err)
	}
}

func TestBackupWhileTargetIsOpen(t *testing.T) {
	dir := t.TempDir()
	src := New(filepath.Join(dir, "source.db"))
	defer func() { _ = src.Close() }()
	if err := src.Append(history.Entry{ID: "source", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append source: %v", err)
	}
	target := filepath.Join(dir, "backup.db")
	old := New(target)
	if err := old.Append(history.Entry{ID: "old-backup", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append old: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old: %v", err)
	}

	db, err := src.handle()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	waiting := db.Stats().WaitCount
	result := make(chan error, 1)
	go func() { result <- src.Backup(target) }()
	deadline := time.Now().Add(5 * time.Second)
	for db.Stats().WaitCount == waiting {
		if time.Now().After(deadline) {
			t.Fatal("backup never waited for the source connection")
		}
		runtime.Gosched()
	}
	active := New(target)
	defer func() { _ = active.Close() }()
	if err := active.Append(history.Entry{ID: "old-active", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append active: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("release conn: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("backup: %v", err)
	}
	restored := New(target)
	defer func() { _ = restored.Close() }()
	es, err := restored.Entries()
	if err != nil || len(es) != 1 || es[0].ID != "source" {
		t.Fatalf("backup holds %+v, %v, want only source", es, err)
	}
}

func TestBackupRefusesNonDatabaseTarget(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	dest := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(dest, []byte("keep me, I am not a database"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	err := s.Backup(dest)
	if err == nil || !strings.Contains(err.Error(), "not a readable SQLite database") {
		t.Fatalf("Backup() error = %v, want a refusal", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "keep me, I am not a database" {
		t.Fatalf("notes changed to %q", b)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, dest, 0o644)
	}
}

func TestBackupOddPathNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow ? in file names")
	}
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	t.Chdir(dir)
	for _, name := range []string{"we?ird #name 100%.db", "relative.db"} {
		if err := s.Backup(name); err != nil {
			t.Fatalf("backup to %q: %v", name, err)
		}
		head := make([]byte, 16)
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("open %q: %v", name, err)
		}
		_, err = io.ReadFull(f, head)
		_ = f.Close()
		if err != nil || string(head) != "SQLite format 3\x00" {
			t.Fatalf("%q is not a SQLite database: %q, %v", name, head, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "we")); !os.IsNotExist(err) {
		t.Fatalf("backup wrote to a truncated path: %v", err)
	}
}

func TestBackupRefusesLiveDBAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	defer func() { _ = s.Close() }()
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	link := filepath.Join(dir, "link.db")
	if err := os.Symlink(p, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := s.Backup(link); err == nil {
		t.Fatal("backup to a link to the live db succeeded")
	}
}

func TestBackupRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	dest := filepath.Join(dir, "Backups")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := s.Backup(dest)
	if err == nil || !strings.Contains(err.Error(), "not a readable SQLite database") {
		t.Fatalf("Backup() error = %v, want a refusal", err)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, dest, 0o755)
	}
}

func TestBackupWaitsForLockedTarget(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	dest := filepath.Join(dir, "b.db")
	old := New(dest)
	if err := old.Load(); err != nil {
		t.Fatalf("load old: %v", err)
	}
	db, err := old.handle()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(200 * time.Millisecond)
		_, _ = conn.ExecContext(context.Background(), "COMMIT")
		_ = conn.Close()
		_ = old.Close()
	}()
	if err := s.Backup(dest); err != nil {
		t.Fatalf("backup: %v", err)
	}
	<-done
}

func TestBackupRemovesFileItCreatedOnError(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	dest := filepath.Join(dir, "b.db")
	// With a directory where the journal goes, SQLite fails after b.db already exists.
	if err := os.Mkdir(dest+"-journal", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := s.Backup(dest); err == nil {
		t.Fatal("backup succeeded without a journal")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("failed backup left %s behind: %v", filepath.Base(dest), err)
	}
}

func TestExportJSONWritesEmptyListForEmptyHistory(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	out := filepath.Join(dir, "out.json")
	if _, err := s.ExportJSON(out); err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != "[]" {
		t.Fatalf("export = %q, %v, want []", got, err)
	}
}
