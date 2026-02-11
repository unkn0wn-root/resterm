package sqlite

import (
	"os"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
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
	if err := s.ensure(); err != nil {
		return Stats{}, err
	}

	st := Stats{Path: s.p}
	var minNS, maxNS int64
	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(MIN(exec_ns), 0), COALESCE(MAX(exec_ns), 0) FROM hist`,
	).Scan(&st.Rows, &minNS, &maxNS); err != nil {
		return Stats{}, errdef.Wrap(errdef.CodeHistory, err, "query history stats")
	}
	st.Oldest = nsToTime(minNS)
	st.Newest = nsToTime(maxNS)

	v, err := schemaVersion(s.db)
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
	if err := s.ensure(); err != nil {
		return err
	}
	return checkDB(s.db, full)
}

func (s *Store) Compact() error {
	if err := s.ensure(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`); err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "checkpoint history db")
	}
	if _, err := s.db.Exec(`VACUUM;`); err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "compact history db")
	}
	if _, err := s.db.Exec(`PRAGMA optimize;`); err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "optimize history db")
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
