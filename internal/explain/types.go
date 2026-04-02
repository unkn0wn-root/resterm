package explain

type Status string

const (
	StatusReady   Status = "ready"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

type StageStatus string

const (
	StageOK      StageStatus = "ok"
	StageSkipped StageStatus = "skipped"
	StageError   StageStatus = "error"
)

type Report struct {
	Name     string
	Method   string
	URL      string
	Env      string
	Status   Status
	Decision string
	Failure  string
	Vars     []Var
	Stages   []Stage
	Final    *Final
	Warnings []string
}

type Stage struct {
	Name    string
	Status  StageStatus
	Summary string
	Changes []Change
	Notes   []string
}

type Change struct {
	Field  string
	Before string
	After  string
}

type Var struct {
	Name     string
	Source   string
	Value    string
	Shadowed []string
	Uses     int
	Missing  bool
	Dynamic  bool
}

type Final struct {
	Mode     string
	Protocol string
	Method   string
	URL      string
	Headers  []Header
	Body     string
	BodyNote string
	Settings []Pair
	Route    *Route
	Details  []Pair
	Steps    []string
}

type Header struct {
	Name  string
	Value string
}

type Pair struct {
	Key   string
	Value string
}

type Route struct {
	Kind    string
	Summary string
	Notes   []string
}
