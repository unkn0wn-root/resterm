package intellisense

type Scope struct {
	Variables         []VarRef
	Environments      []string
	EnvironmentGroups map[string][]string
	Profiles          ProfileSet
	RequestNames      []string
}

type VarRef struct {
	Name   string
	Origin string
	Secret bool
}

type ProfileSet struct {
	Patch []string
	SSH   []string
	K8s   []string
}
