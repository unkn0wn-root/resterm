package curl

type Cmd struct {
	Segs []Seg
}

type Seg struct {
	Items []Item
	Unk   []string
}

type Item struct {
	Opt   Opt
	Pos   string
	IsOpt bool
}

type Opt struct {
	Key string
	Val string
}
