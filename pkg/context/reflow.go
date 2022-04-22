package context

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v43/github"
)

type ReflowBuilder struct {
	Client *github.Client
}

func (rb *ReflowBuilder) Build(ctx context.Context, m map[string]any) error {
	event, err := Get[string](m, "github.event_name")
	if err != nil {
		return fmt.Errorf("git builder: %w", err)
	}

	s, err := Get[string](m, "github.repository")
	if err != nil {
		return fmt.Errorf("%s: git builder: %w", event, err)
	}

	v := strings.Split(s, "/")
	if len(v) < 2 || v[0] == "" || v[1] == "" {
		return fmt.Errorf("%s: git builder: invalid repository: %q", event, s)
	}

	owner, repo := v[0], v[1]

	Set(m, "reflow.owner", owner)
	Set(m, "reflow.repo", repo)

	switch event {
	case "issue_comment":
		err = rb.buildIssueComment(ctx, m, owner, repo)
	case "push":
		err = rb.buildPush(ctx, m, owner, repo)
	case "pull_request":
		err = rb.buildPullRequest(ctx, m, owner, repo)
	default:
		return fmt.Errorf("unsupported event type: %q", event)
	}

	if err != nil {
		return fmt.Errorf("%s: git builder: %w", event, err)
	}

	return nil
}

func (rb *ReflowBuilder) buildIssueComment(ctx context.Context, m map[string]any, owner, repo string) error {
	if v, err := Get[map[string]any](m, "github.event.issue.pull_request"); err != nil || len(v) == 0 {
		return errors.New("issue is not a pull request")
	}

	num, err := Get[int](m, "github.event.issue.number")
	if err != nil {
		return err
	}

	pr, _, err := rb.Client.PullRequests.Get(ctx, owner, repo, num)
	if err != nil {
		return fmt.Errorf("error getting pull request: %w", err)
	}

	var ref, sha string = *pr.Head.Ref, *pr.Head.SHA

	Set(m, "reflow.ref", ref)
	Set(m, "reflow.sha", sha)

	return nil
}

func (rb *ReflowBuilder) buildPush(ctx context.Context, m map[string]any, owner, repo string) error {
	ref, err := Get[string](m, "github.ref")
	if err != nil {
		return err
	}

	sha, err := Get[string](m, "github.sha")
	if err != nil {
		return err
	}

	Set(m, "reflow.ref", ref)
	Set(m, "reflow.sha", sha)

	return nil
}

func (rb *ReflowBuilder) buildPullRequest(ctx context.Context, m map[string]any, owner, repo string) error {
	ref, err := Get[string](m, "github.event.pull_request.head.ref")
	if err != nil {
		return err
	}

	sha, err := Get[string](m, "github.event.pull_request.head.sha")
	if err != nil {
		return err
	}

	Set(m, "reflow.ref", ref)
	Set(m, "reflow.sha", sha)

	return nil
}
