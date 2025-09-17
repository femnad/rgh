package main

import (
	"context"
	"log"

	"github.com/alexflint/go-arg"
	"github.com/femnad/rgh/gh"
	"github.com/femnad/rgh/internal"
	"github.com/femnad/rgh/run"
)

var args struct {
	Commit   bool              `arg:"-c,--commit"`
	Inputs   map[string]string `arg:"-i,--inputs"`
	Open     bool              `arg:"-o,--open"`
	Push     bool              `arg:"-p,--push"`
	Print    bool              `arg:"-p,--print"`
	Ref      string            `arg:"-e,--ref"`
	Repo     string            `arg:"-r,--repo"`
	Watch    bool              `arg:"-w,--watch"`
	Workflow string            `arg:"positional,required"`
}

func runWorkflow() error {
	arg.MustParse(&args)

	opts := internal.Options{
		Commit: args.Commit,
		Open:   args.Open,
		Print:  args.Print,
		Push:   args.Push,
		Watch:  args.Watch,
	}

	runSpec, err := run.GetSpec(opts, args.Repo, args.Ref, args.Workflow, args.Inputs)
	if err != nil {
		return err
	}

	return gh.Run(context.Background(), opts, runSpec)
}

func main() {
	if err := runWorkflow(); err != nil {
		log.Fatal(err)
	}
}
