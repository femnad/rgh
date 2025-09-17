package git

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	marecmd "github.com/femnad/mare/cmd"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/femnad/rgh/internal"
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

func GetRepo(repoPath string) (*git.Repository, error) {
	if repoPath == "/" {
		return nil, fmt.Errorf(rootNotFoundError)
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		if err.Error() == "repository does not exists" {
			parent, _ := path.Split(repoPath)
			return GetRepo(parent)
		} else {
			return nil, err
		}
	}

	return repo, nil
}

func GetRepoID(repo *git.Repository) (string, error) {
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

func GetRepoRef(repo *git.Repository, refOverride string) (string, error) {
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

func MaybeUpdateRepo(options internal.Options, repo *git.Repository) error {
	if !options.Commit {
		return nil
	}

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

	if !options.Push {
		return nil
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
