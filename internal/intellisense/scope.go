package intellisense

type Scope struct {
	Variables    []VarRef
	Environments []string
	Profiles     ProfileSet
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
