package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/femnad/rgh/internal"
)

const (
	maxWorkflowWaitAttempts = 5
)

var (
	workflowFileRegex = regexp.MustCompile(".*\\.y(a)?ml")
	workflowPathRegex = regexp.MustCompile(".github/workflows/(.*)\\.y(a)?ml")
)

type runViewResp struct {
	Url string `json:"html_url"`
}

type workflowListResp struct {
	Workflows []struct {
		Path string `json:"path"`
	} `json:"workflows"`
}

type workflowRunReq struct {
	Ref    string            `json:"ref,omitempty"`
	Inputs map[string]string `json:"inputs,omitempty"`
}

type workflowRunsResp struct {
	TotalCount   int `json:"total_count"`
	WorkflowRuns []struct {
		Id        int       `json:"id"`
		Url       string    `json:"url"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"workflow_runs"`
}

func findMatchingWorkflow(client *api.RESTClient, repo string) (string, error) {
	apiPath := fmt.Sprintf("repos/%s/actions/workflows", repo)
	var resp workflowListResp
	err := client.Get(apiPath, &resp)
	if err != nil {
		return "", err
	}

	for _, workflow := range resp.Workflows {
		matches := workflowPathRegex.FindStringSubmatch(workflow.Path)
		if len(matches) == 0 {
			continue
		}

		_, basePath := path.Split(workflow.Path)
		return basePath, nil
	}

	return "", errors.New("unable to find matching workflow")
}

func Run(ctx context.Context, options internal.Options, spec internal.RunSpec) error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}

	body, err := json.Marshal(workflowRunReq{Ref: spec.Ref, Inputs: spec.Inputs})
	if err != nil {
		return err
	}

	workflow := spec.Workflow
	if !workflowFileRegex.MatchString(workflow) {
		workflow, err = findMatchingWorkflow(client, spec.Repo)
		if err != nil {
			return err
		}
	}

	bodyReader := bytes.NewReader(body)
	apiPath := fmt.Sprintf("repos/%s/actions/workflows/%s/dispatches", spec.Repo, workflow)

	now := time.Now()
	err = client.Post(apiPath, bodyReader, struct{}{})
	if err != nil {
		return fmt.Errorf("error sending workflow dispatch request: %w", err)
	}

	var attempts, id int
	var runsResp workflowRunsResp
	apiPath = fmt.Sprintf("repos/%s/actions/workflows/%s/runs", spec.Repo, workflow)
	sleep := time.Second * 1
	for {
		if attempts > maxWorkflowWaitAttempts {
			return fmt.Errorf("max workflow wait attempts reached")
		}

		err = client.Get(apiPath, &runsResp)
		if err != nil {
			return fmt.Errorf("error finding workflow run: %w", err)
		}

		if runsResp.TotalCount == 0 {
			return fmt.Errorf("unable to find any workflow runs")
		}

		latest := runsResp.WorkflowRuns[0]
		if latest.CreatedAt.After(now) {
			id = latest.Id
			break
		}

		time.Sleep(sleep)
		sleep *= 2
		attempts++
	}

	var run runViewResp
	apiPath = fmt.Sprintf("repos/%s/actions/runs/%d", spec.Repo, id)
	err = client.Get(apiPath, &run)
	if err != nil {
		return fmt.Errorf("error viewing run: %w", err)
	}

	if options.Print {
		fmt.Printf("%s\n", run.Url)
	}
	if options.Open {
		cmd := exec.Command("xdg-open", run.Url)
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("error opening URL: %w", err)
		}
	}

	if options.Watch {
		idStr := strconv.Itoa(id)
		return gh.ExecInteractive(ctx, "run", "watch", "-R", spec.Repo, idStr)
	}

	return nil
}
