package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

const stateFileVersion = 1

type statePaths struct {
	Root    string
	Runtime string
	Auth    string
	History string
}

type runtimeStateFile struct {
	Version int                 `json:"version"`
	State   engine.RuntimeState `json:"state"`
}

type authStateFile struct {
	Version int              `json:"version"`
	State   engine.AuthState `json:"state"`
}

func resolveStatePaths(opts Options) (statePaths, error) {
	if !usesStateDir(opts) {
		return statePaths{}, nil
	}
	root := str.Trim(opts.StateDir)
	if root == "" {
		root = filepath.Join(config.Dir(), "runner")
	}
	root, err := absCleanPath(root)
	if err != nil {
		return statePaths{}, err
	}
	return statePaths{
		Root:    root,
		Runtime: filepath.Join(root, "runtime.json"),
		Auth:    filepath.Join(root, "auth.json"),
		History: filepath.Join(root, "history.db"),
	}, nil
}

func usesStateDir(opts Options) bool {
	return opts.History || opts.PersistGlobals || opts.PersistAuth
}

func openHistoryStore(paths statePaths, opts Options) history.Store {
	if !opts.History || str.Trim(paths.History) == "" {
		return nil
	}
	return histdb.New(paths.History)
}

func loadRunnerState(h engine.Executor, paths statePaths, opts Options) error {
	if opts.PersistGlobals {
		state, err := readRuntimeState(paths.Runtime)
		if err != nil {
			return err
		}
		h.LoadRuntimeState(state)
	}
	if opts.PersistAuth {
		state, err := readAuthState(paths.Auth)
		if err != nil {
			return err
		}
		h.LoadAuthState(state)
	}
	return nil
}

func saveRunnerState(h engine.Executor, paths statePaths, opts Options) error {
	if !usesStateDir(opts) {
		return nil
	}
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return err
	}
	if opts.PersistGlobals {
		if err := writeRuntimeState(paths.Runtime, h.RuntimeState()); err != nil {
			return err
		}
	}
	if opts.PersistAuth {
		if err := writeAuthState(paths.Auth, h.AuthState()); err != nil {
			return err
		}
	}
	return nil
}

func readRuntimeState(path string) (engine.RuntimeState, error) {
	var file runtimeStateFile
	if err := readStateFile(path, &file); err != nil {
		return engine.RuntimeState{}, err
	}
	return file.State, nil
}

func readAuthState(path string) (engine.AuthState, error) {
	var file authStateFile
	if err := readStateFile(path, &file); err != nil {
		return engine.AuthState{}, err
	}
	return file.State, nil
}

func readStateFile(path string, dst any) error {
	path = str.Trim(path)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("read state file %s: %w", path, err)
	}
	return nil
}

func writeRuntimeState(path string, state engine.RuntimeState) error {
	return writeStateFile(path, runtimeStateFile{
		Version: stateFileVersion,
		State:   state,
	})
}

func writeAuthState(path string, state engine.AuthState) error {
	return writeStateFile(path, authStateFile{
		Version: stateFileVersion,
		State:   state,
	})
}

func writeStateFile(path string, state any) error {
	path = str.Trim(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
