package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitWorker struct {
	basePath string
}

func NewGitWorker(basePath string) *GitWorker {
	return &GitWorker{basePath: basePath}
}

func (w *GitWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "clone", Description: "Clone a git repository"},
		{Name: "status", Description: "Get git status"},
		{Name: "log", Description: "Get commit history"},
		{Name: "diff", Description: "Get diff of changes"},
		{Name: "commit", Description: "Create a commit"},
		{Name: "push", Description: "Push to remote"},
		{Name: "pull", Description: "Pull from remote"},
		{Name: "branch", Description: "Manage branches"},
		{Name: "checkout", Description: "Checkout a branch or commit"},
	}
}

func (w *GitWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "clone", "git_clone":
		return w.clone(ctx, input)
	case "status", "git_status":
		return w.status(ctx, input)
	case "log", "git_log":
		return w.log(ctx, input)
	case "diff", "git_diff":
		return w.diff(ctx, input)
	case "commit", "git_commit":
		return w.commit(ctx, input)
	case "push", "git_push":
		return w.push(ctx, input)
	case "pull", "git_pull":
		return w.pull(ctx, input)
	case "branch", "git_branch":
		return w.branch(ctx, input)
	case "checkout", "git_checkout":
		return w.checkout(ctx, input)
	default:
		return nil, nil
	}
}

type CloneInput struct {
	URL    string `json:"url"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

func (w *GitWorker) clone(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req CloneInput
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	path := req.Path
	if path == "" {
		parts := strings.Split(req.URL, "/")
		name := parts[len(parts)-1]
		path = strings.TrimSuffix(name, ".git")
	}

	args := []string{"clone"}
	if req.Branch != "" {
		args = append(args, "-b", req.Branch)
	}
	args = append(args, req.URL, path)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = w.basePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"status": "cloned",
		"path":   path,
	})
}

type StatusInput struct {
	Repo string `json:"repo"`
}

func (w *GitWorker) status(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req StatusInput
	json.Unmarshal(input, &req)

	repoPath := w.resolveRepoPath(req.Repo)
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	var changes []map[string]string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		changes = append(changes, map[string]string{
			"status": line[:2],
			"file":   strings.TrimSpace(line[3:]),
		})
	}

	return json.Marshal(map[string]interface{}{
		"repo":    req.Repo,
		"changes": changes,
	})
}

type LogInput struct {
	Repo   string `json:"repo"`
	Count  int    `json:"count"`
	Branch string `json:"branch"`
}

func (w *GitWorker) log(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req LogInput
	json.Unmarshal(input, &req)

	count := req.Count
	if count == 0 {
		count = 10
	}

	repoPath := w.resolveRepoPath(req.Repo)
	args := []string{"log", fmt.Sprintf("-%d", count), "--pretty=format:%h|%s|%an|%ai"}
	if req.Branch != "" {
		args = append(args, req.Branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	var commits []map[string]string
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, map[string]string{
				"hash":    parts[0],
				"message": parts[1],
				"author":  parts[2],
				"date":    parts[3],
			})
		}
	}

	return json.Marshal(commits)
}

type DiffInput struct {
	Repo   string `json:"repo"`
	File   string `json:"file"`
	Target string `json:"target"`
}

func (w *GitWorker) diff(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DiffInput
	json.Unmarshal(input, &req)

	repoPath := w.resolveRepoPath(req.Repo)
	args := []string{"diff"}
	if req.Target != "" {
		args = append(args, req.Target)
	}
	if req.File != "" {
		args = append(args, "--", req.File)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"repo": req.Repo,
		"diff": string(out),
	})
}

type CommitInput struct {
	Repo    string   `json:"repo"`
	Message string   `json:"message"`
	Files   []string `json:"files"`
}

func (w *GitWorker) commit(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req CommitInput
	json.Unmarshal(input, &req)

	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	repoPath := w.resolveRepoPath(req.Repo)

	if len(req.Files) > 0 {
		for _, f := range req.Files {
			cmd := exec.CommandContext(ctx, "git", "add", f)
			cmd.Dir = repoPath
			if out, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("git add %s: %s", f, string(out))
			}
		}
	} else {
		cmd := exec.CommandContext(ctx, "git", "add", "-A")
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git add: %s", string(out))
		}
	}

	cmd := exec.CommandContext(ctx, "git", "commit", "-m", req.Message)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"status":  "committed",
		"message": req.Message,
	})
}

type PushInput struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Force  bool   `json:"force"`
}

func (w *GitWorker) push(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req PushInput
	json.Unmarshal(input, &req)

	repoPath := w.resolveRepoPath(req.Repo)
	args := []string{"push"}
	if req.Force {
		args = append(args, "-f")
	}
	if req.Branch != "" {
		args = append(args, "origin", req.Branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"status": "pushed",
		"repo":   req.Repo,
	})
}

type PullInput struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

func (w *GitWorker) pull(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req PullInput
	json.Unmarshal(input, &req)

	repoPath := w.resolveRepoPath(req.Repo)
	args := []string{"pull", "origin"}
	if req.Branch != "" {
		args = append(args, req.Branch)
	} else {
		args = append(args, "HEAD")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"status": "pulled",
		"repo":   req.Repo,
		"output": string(out),
	})
}

type BranchInput struct {
	Repo   string `json:"repo"`
	Name   string `json:"name"`
	Action string `json:"action"`
	Delete bool   `json:"delete"`
}

func (w *GitWorker) branch(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req BranchInput
	json.Unmarshal(input, &req)

	repoPath := w.resolveRepoPath(req.Repo)

	action := req.Action
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		cmd := exec.CommandContext(ctx, "git", "branch", "-a")
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		var branches []string
		for _, b := range strings.Split(string(out), "\n") {
			b = strings.TrimSpace(b)
			if b != "" {
				branches = append(branches, b)
			}
		}
		return json.Marshal(map[string]interface{}{
			"repo":     req.Repo,
			"branches": branches,
		})

	case "create":
		if req.Name == "" {
			return nil, fmt.Errorf("branch name is required")
		}
		cmd := exec.CommandContext(ctx, "git", "checkout", "-b", req.Name)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s: %s", err, string(out))
		}
		return json.Marshal(map[string]string{
			"status": "created",
			"branch": req.Name,
		})

	case "delete":
		if req.Name == "" {
			return nil, fmt.Errorf("branch name is required")
		}
		args := []string{"branch", "-d"}
		if req.Delete {
			args = []string{"branch", "-D"}
		}
		args = append(args, req.Name)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s: %s", err, string(out))
		}
		return json.Marshal(map[string]string{
			"status": "deleted",
			"branch": req.Name,
		})

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

type CheckoutInput struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Create bool   `json:"create"`
}

func (w *GitWorker) checkout(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req CheckoutInput
	json.Unmarshal(input, &req)

	if req.Branch == "" {
		return nil, fmt.Errorf("branch is required")
	}

	repoPath := w.resolveRepoPath(req.Repo)
	args := []string{"checkout"}
	if req.Create {
		args = append(args, "-b")
	}
	args = append(args, req.Branch)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]string{
		"status": "checked out",
		"branch": req.Branch,
	})
}

func (w *GitWorker) resolveRepoPath(repo string) string {
	if repo == "" {
		return w.basePath
	}
	if filepath.IsAbs(repo) {
		return repo
	}
	return filepath.Join(w.basePath, repo)
}

func init() {
	_ = time.Time{}
	_ = bytes.Buffer{}
	_ = os.ErrNotExist
}
