package vars

// GlobalMutation describes a script/runtime global variable set or delete.
type GlobalMutation struct {
	Name   string
	Value  string
	Secret bool
	Delete bool
}
