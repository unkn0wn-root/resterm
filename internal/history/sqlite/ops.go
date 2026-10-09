package sqlite

import (
	"os"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
)

func (s *Store) Stats() (history.Stats, error) {
	db, err := s.handle()
	if err != nil {
		return history.Stats{}, err
	}

	st := history.Stats{Path: s.p}
	var minNS, maxNS int64
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(MIN(exec_ns), 0), COALESCE(MAX(exec_ns), 0) FROM hist`,
	).Scan(&st.Rows, &minNS, &maxNS); err != nil {
		return history.Stats{}, diag.WrapAs(diag.ClassHistory, err, "query history stats")
	}
	st.Oldest = nsToTime(minNS)
	st.Newest = nsToTime(maxNS)

	v, err := schemaVersion(db)
	if err != nil {
		return history.Stats{}, err
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
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "checkpoint history db")
	}
	if _, err := db.Exec(`VACUUM;`); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "compact history db")
	}
	if _, err := db.Exec(`PRAGMA optimize;`); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "optimize history db")
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
