package main

import (
	"context"
	"log"

	"github.com/alexflint/go-arg"
	"github.com/femnad/rgh/gh"
	"github.com/femnad/rgh/run"
)

var args struct {
	Inputs   map[string]string `arg:"-i,--inputs"`
	Open     bool              `arg:"-o,--open"`
	Print    bool              `arg:"-p,--print"`
	Ref      string            `arg:"-e,--ref"`
	Repo     string            `arg:"-r,--repo"`
	Watch    bool              `arg:"-w,--watch"`
	Workflow string            `arg:"positional,required"`
}

func runWorkflow() error {
	arg.MustParse(&args)
	spec, err := run.GetRunSpec(args.Repo, args.Ref, args.Workflow, args.Inputs)
	if err != nil {
		return err
	}

	opt := gh.Options{
		Open:  args.Open,
		Print: args.Print,
		Watch: args.Watch,
	}
	return gh.Run(context.Background(), spec, opt)
}

func main() {
	if err := runWorkflow(); err != nil {
		log.Fatal(err)
	}
}
