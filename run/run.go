package run

import (
	"os"

	rghgit "github.com/femnad/rgh/git"
	"github.com/femnad/rgh/internal"
	"github.com/go-git/go-git/v5"
)

func GetSpec(options internal.Options, repo, ref, workflow string, inputs map[string]string) (
	spec internal.RunSpec, err error) {
	pwd, err := os.Getwd()
	if err != nil {
		return
	}

	var gitRepo *git.Repository
	if repo == "" {
		gitRepo, err = rghgit.GetRepo(pwd)
		if err != nil {
			return
		}

		repo, err = rghgit.GetRepoID(gitRepo)
		if err != nil {
			return
		}
	}

	if ref == "" {
		if gitRepo == nil {
			gitRepo, err = rghgit.GetRepo(pwd)
			if err != nil {
				return
			}
		}

		ref, err = rghgit.GetRepoRef(gitRepo, ref)
		if err != nil {
			return
		}
	}

	if gitRepo != nil {
		err = rghgit.MaybeUpdateRepo(options, gitRepo)
		if err != nil {
			return
		}
	}

	return internal.RunSpec{
		Inputs:   inputs,
		Ref:      ref,
		Repo:     repo,
		Workflow: workflow,
	}, nil
}
