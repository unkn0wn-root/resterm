package headless

import "time"

// EnvironmentSet maps environment names to variable values.
type EnvironmentSet map[string]map[string]string

type EnvironmentSelection map[string]string

type EnvironmentGroup struct {
	Default  string         `json:"default,omitempty"`
	Profiles EnvironmentSet `json:"profiles,omitempty"`
}

type EnvironmentGroups map[string]EnvironmentGroup

type GroupedEnvironmentSet struct {
	Shared map[string]string `json:"shared,omitempty"`
	Groups EnvironmentGroups `json:"groups,omitempty"`
}

// Source identifies the request document to execute.
// Path is required and provides the logical file identity and relative-resolution anchor.
// Path may be synthetic when Content is provided.
// Content overrides the bytes loaded from Path when provided.
type Source struct {
	Path    string `json:"path,omitempty"`
	Content []byte `json:"-"`
}

// Options configures a headless run.
type Options struct {
	Version       string             `json:"version,omitempty"`
	Source        Source             `json:"source,omitempty"`
	WorkspaceRoot string             `json:"workspaceRoot,omitempty"`
	Recursive     bool               `json:"recursive,omitempty"`
	State         StateOptions       `json:"state,omitempty"`
	FailFast      bool               `json:"failFast,omitempty"`
	Environment   EnvironmentOptions `json:"environment,omitempty"`
	Compare       CompareOptions     `json:"compare,omitempty"`
	Profile       ProfileOptions     `json:"profile,omitempty"`
	HTTP          HTTPOptions        `json:"http,omitempty"`
	GRPC          GRPCOptions        `json:"grpc,omitempty"`
	Selection     Selection          `json:"selection,omitempty"`
}

// StateOptions controls artifacts and persisted runtime state.
type StateOptions struct {
	ArtifactDir    string `json:"artifactDir,omitempty"`
	StateDir       string `json:"stateDir,omitempty"`
	PersistGlobals bool   `json:"persistGlobals,omitempty"`
	PersistAuth    bool   `json:"persistAuth,omitempty"`
	History        bool   `json:"history,omitempty"`
}

// EnvironmentOptions controls environment loading and selection.
type EnvironmentOptions struct {
	Set       EnvironmentSet         `json:"set,omitempty"`
	Grouped   *GroupedEnvironmentSet `json:"grouped,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Selection EnvironmentSelection   `json:"selection,omitempty"`
	FilePath  string                 `json:"filePath,omitempty"`
}

// CompareOptions configures compare runs across multiple environments.
type CompareOptions struct {
	Targets []string `json:"targets,omitempty"`
	Base    string   `json:"base,omitempty"`
	Group   string   `json:"group,omitempty"`
}

// ProfileOptions configures profile runs.
type ProfileOptions struct {
	Enabled bool `json:"enabled,omitempty"`
}

// Selection narrows which request or workflow to run.
type Selection struct {
	Request  string `json:"request,omitempty"`
	Workflow string `json:"workflow,omitempty"`
	Tag      string `json:"tag,omitempty"`
	All      bool   `json:"all,omitempty"`
}

// HTTPOptions configures default HTTP client behavior.
type HTTPOptions struct {
	Timeout            time.Duration `json:"timeout,omitempty"`
	FollowRedirects    *bool         `json:"followRedirects,omitempty"`
	MaxRedirects       *int          `json:"maxRedirects,omitempty"`
	MaxResponseBytes   *int64        `json:"maxResponseBytes,omitempty"`
	InsecureSkipVerify bool          `json:"insecureSkipVerify,omitempty"`
	ProxyURL           string        `json:"proxyURL,omitempty"`
}

// GRPCOptions configures default gRPC behavior.
type GRPCOptions struct {
	Plaintext *bool `json:"plaintext,omitempty"`
}
