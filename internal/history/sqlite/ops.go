package sqlite

import (
	"errors"
	"os"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type Stats struct {
	Path     string
	Schema   int
	Rows     int64
	Oldest   time.Time
	Newest   time.Time
	DBBytes  int64
	WALBytes int64
	SHMBytes int64
}

func (s *Store) Stats() (Stats, error) {
	db, err := s.handle()
	if err != nil {
		return Stats{}, err
	}

	st := Stats{Path: s.p}
	var minNS, maxNS int64
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(MIN(exec_ns), 0), COALESCE(MAX(exec_ns), 0) FROM hist`,
	).Scan(&st.Rows, &minNS, &maxNS); err != nil {
		return Stats{}, diag.WrapAs(diag.ClassHistory, err, "query history stats")
	}
	st.Oldest = nsToTime(minNS)
	st.Newest = nsToTime(maxNS)

	v, err := schemaVersion(db)
	if err != nil {
		return Stats{}, err
	}
	st.Schema = v
	st.DBBytes = fileSize(s.p)
	st.WALBytes = fileSize(s.p + "-wal")
	st.SHMBytes = fileSize(s.p + "-shm")
	return st, nil
}

func (s *Store) Check(full bool) error {
	db, err := s.handle()
	if err != nil {
		return err
	}
	return checkDB(db, full)
}

func (s *Store) Compact() error {
	db, err := s.handle()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`VACUUM;`); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "compact history db")
	}
	if _, err := db.Exec(`PRAGMA optimize;`); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "optimize history db")
	}
	// In WAL mode VACUUM goes through the WAL. The file itself only shrinks at the checkpoint.
	var busy, logFrames, checkpointed int
	if err := db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE);`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "checkpoint history db")
	}
	if busy != 0 {
		return diag.WrapAs(
			diag.ClassHistory,
			errors.New("another process is using the history db, so its file did not shrink. Try again later"),
			"checkpoint history db",
		)
	}
	return nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
