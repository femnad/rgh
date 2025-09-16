package run

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	marecmd "github.com/femnad/mare/cmd"
	"github.com/femnad/rgh/gh"
)

const (
	rootNotFoundError = "repo root not found"
)

var (
	githubRemoteRegex = regexp.MustCompile("git@github.com:([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)(.git)?")
)

type baseAuthor struct {
	email string
	name  string
}

func getRepo(repoPath string) (*git.Repository, error) {
	if repoPath == "/" {
		return nil, fmt.Errorf(rootNotFoundError)
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		if err.Error() == "repository does not exists" {
			parent, _ := path.Split(repoPath)
			return getRepo(parent)
		} else {
			return nil, err
		}
	}

	return repo, nil
}

func getRepoID(repo *git.Repository) (string, error) {
	var remotes []*git.Remote
	remotes, err := repo.Remotes()
	if err != nil {
		return "", err
	}

	for _, remote := range remotes {
		for _, url := range remote.Config().URLs {
			if !githubRemoteRegex.MatchString(url) {
				continue
			}
			matches := githubRemoteRegex.FindStringSubmatch(url)
			if len(matches) == 0 {
				continue
			}
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("unable to determine repo ref")
}

func getRepoRef(repo *git.Repository, refOverride string) (string, error) {
	if refOverride != "" {
		return refOverride, nil
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("unable to determine repo HEAD: %s", err)
	}

	if head.Name().IsBranch() {
		headRef := head.Name().String()
		return strings.TrimPrefix(headRef, "refs/heads/"), nil
	}
	return head.Hash().String(), nil
}

// lookupGitConfig is a hack got getting config values which could be set by include statements
func lookupGitConfig(key string) (string, error) {
	out, err := marecmd.RunFmtErr(marecmd.Input{Command: fmt.Sprintf("git config %s", key)})
	if err != nil {
		return "", err
	}

	return out.Stdout, nil
}

func getAuthor(repo *git.Repository) (author baseAuthor, err error) {
	cfg, err := repo.Config()
	if err != nil {
		return
	}

	email := cfg.User.Email
	name := cfg.User.Name
	if email == "" {
		email, err = lookupGitConfig("user.email")
		if err != nil {
			return
		}
	}
	if name == "" {
		name, err = lookupGitConfig("user.name")
		if err != nil {
			return
		}
	}

	return baseAuthor{email: email, name: name}, err
}

func getIdentityAgent() (string, error) {
	f, err := os.Open(os.ExpandEnv("$HOME/.ssh/config"))
	if err != nil {
		return "", err
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return "", err
	}

	return cfg.Get("github.com", "IdentityAgent")
}

func getAuth() (callback gitssh.PublicKeysCallback, err error) {
	identityAgent, err := getIdentityAgent()
	if err != nil {
		return
	}

	identityAgent = strings.Replace(identityAgent, "~", os.Getenv("HOME"), 1)
	conn, err := net.Dial("unix", identityAgent)
	if err != nil {
		return
	}

	ag := agent.NewClient(conn)
	return gitssh.PublicKeysCallback{
		User: "git",
		Callback: func() (signers []ssh.Signer, err error) {
			return ag.Signers()
		},
	}, nil
}

func maybeCommit(repo *git.Repository) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	status, err := worktree.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		return nil
	}

	err = worktree.AddWithOptions(&git.AddOptions{All: true})
	if err != nil {
		return err
	}

	fmt.Print("Commit message: ")
	var msg string
	msg, err = bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}

	author, err := getAuthor(repo)
	if err != nil {
		return err
	}

	gitAuthor := object.Signature{
		Name:  author.name,
		Email: author.email,
		When:  time.Now(),
	}

	_, err = worktree.Commit(msg, &git.CommitOptions{Author: &gitAuthor})
	if err != nil {
		return err
	}

	auth, err := getAuth()
	if err != nil {
		return err
	}

	err = repo.Push(&git.PushOptions{Auth: &auth})
	if err != nil {
		return fmt.Errorf("error pushing changes: %s", err)
	}

	return nil
}

func GetRunSpec(repo, ref, workflow string, inputs map[string]string) (spec gh.RunSpec, err error) {
	pwd, err := os.Getwd()
	if err != nil {
		return
	}

	var gitRepo *git.Repository
	if repo == "" {
		gitRepo, err = getRepo(pwd)
		if err != nil {
			return
		}

		repo, err = getRepoID(gitRepo)
		if err != nil {
			return
		}
	}

	if ref == "" {
		if gitRepo == nil {
			gitRepo, err = getRepo(pwd)
			if err != nil {
				return
			}
		}

		ref, err = getRepoRef(gitRepo, ref)
		if err != nil {
			return
		}
	}

	if gitRepo != nil {
		err = maybeCommit(gitRepo)
		if err != nil {
			return
		}
	}

	return gh.RunSpec{
		Inputs:   inputs,
		Ref:      ref,
		Repo:     repo,
		Workflow: workflow,
	}, nil
}
