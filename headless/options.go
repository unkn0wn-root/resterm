package headless

import "time"

// EnvironmentSet maps environment names to variable values.
type EnvironmentSet map[string]map[string]string

// Options configures a headless run.
type Options struct {
	Version       string             `json:"version,omitempty"`
	FilePath      string             `json:"filePath,omitempty"`
	FileData      []byte             `json:"-"`
	WorkspaceRoot string             `json:"workspaceRoot,omitempty"`
	Recursive     bool               `json:"recursive,omitempty"`
	State         StateOptions       `json:"state,omitempty"`
	Environment   EnvironmentOptions `json:"environment,omitempty"`
	Compare       CompareOptions     `json:"compare,omitempty"`
	Profile       bool               `json:"profile,omitempty"`
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
	Set      EnvironmentSet `json:"set,omitempty"`
	Name     string         `json:"name,omitempty"`
	FilePath string         `json:"filePath,omitempty"`
}

// CompareOptions configures compare runs across multiple environments.
type CompareOptions struct {
	Targets []string `json:"targets,omitempty"`
	Base    string   `json:"base,omitempty"`
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
	InsecureSkipVerify bool          `json:"insecureSkipVerify,omitempty"`
	ProxyURL           string        `json:"proxyURL,omitempty"`
}

// GRPCOptions configures default gRPC behavior.
type GRPCOptions struct {
	Plaintext *bool `json:"plaintext,omitempty"`
}
