package internal

type Options struct {
	Commit bool
	Push   bool
	Print  bool
	Open   bool
	Watch  bool
}

type RunSpec struct {
	Inputs   map[string]string
	Ref      string
	Repo     string
	Workflow string
}
