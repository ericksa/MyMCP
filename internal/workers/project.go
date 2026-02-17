package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ProjectWorker struct {
	basePath     string
	templatesDir string
}

func NewProjectWorker(basePath, templatesDir string) *ProjectWorker {
	return &ProjectWorker{
		basePath:     basePath,
		templatesDir: templatesDir,
	}
}

func (w *ProjectWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "list_templates", Description: "List available project templates"},
		{Name: "create", Description: "Create project from template"},
		{Name: "info", Description: "Get project information"},
		{Name: "build", Description: "Build the project"},
		{Name: "test", Description: "Run project tests"},
		{Name: "deps", Description: "Manage dependencies"},
		{Name: "structure", Description: "Get project structure"},
	}
}

func (w *ProjectWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "list_templates", "project_list_templates":
		return w.listTemplates(ctx, input)
	case "create", "project_create":
		return w.create(ctx, input)
	case "info", "project_info":
		return w.info(ctx, input)
	case "build", "project_build":
		return w.build(ctx, input)
	case "test", "project_test":
		return w.test(ctx, input)
	case "deps", "project_deps":
		return w.deps(ctx, input)
	case "structure", "project_structure":
		return w.structure(ctx, input)
	default:
		return nil, nil
	}
}

type ListTemplatesInput struct{}

func (w *ProjectWorker) listTemplates(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ListTemplatesInput
	json.Unmarshal(input, &req)

	path := w.templatesDir
	if path == "" {
		path = filepath.Join(w.basePath, "templates")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var templates []map[string]interface{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		templatePath := filepath.Join(path, e.Name())
		info, _ := e.Info()
		templates = append(templates, map[string]interface{}{
			"name":     e.Name(),
			"path":     templatePath,
			"mod_time": info.ModTime(),
		})
	}

	return json.Marshal(map[string]interface{}{
		"templates": templates,
		"count":     len(templates),
	})
}

type CreateInput struct {
	Template string            `json:"template"`
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	Vars     map[string]string `json:"vars"`
}

func (w *ProjectWorker) create(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req CreateInput
	json.Unmarshal(input, &req)

	if req.Name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	templatePath := w.templatesDir
	if templatePath == "" {
		templatePath = filepath.Join(w.basePath, "templates")
	}
	if req.Template != "" {
		templatePath = filepath.Join(templatePath, req.Template)
	}

	destPath := req.Path
	if destPath == "" {
		destPath = filepath.Join(w.basePath, req.Name)
	}

	if _, err := os.Stat(destPath); err == nil {
		return nil, fmt.Errorf("project already exists: %s", req.Name)
	}

	if err := copyDir(templatePath, destPath); err != nil {
		return nil, err
	}

	if len(req.Vars) > 0 {
		applyVars(destPath, req.Vars)
	}

	return json.Marshal(map[string]string{
		"status":   "created",
		"name":     req.Name,
		"path":     destPath,
		"template": req.Template,
	})
}

type InfoInput struct {
	Path string `json:"path"`
}

func (w *ProjectWorker) info(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req InfoInput
	json.Unmarshal(input, &req)

	projectPath := req.Path
	if projectPath == "" {
		projectPath = w.basePath
	}

	info, err := os.Stat(projectPath)
	if err != nil {
		return nil, err
	}

	projectName := filepath.Base(projectPath)
	projectType := detectProjectType(projectPath)

	return json.Marshal(map[string]interface{}{
		"name":     projectName,
		"path":     projectPath,
		"type":     projectType,
		"is_dir":   info.IsDir(),
		"modified": info.ModTime(),
	})
}

type BuildInput struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

func (w *ProjectWorker) build(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req BuildInput
	json.Unmarshal(input, &req)

	projectPath := req.Path
	if projectPath == "" {
		projectPath = w.basePath
	}

	projectType := detectProjectType(projectPath)

	var cmd *exec.Cmd
	switch projectType {
	case "go":
		cmd = exec.CommandContext(ctx, "go", "build", "-o", "bin/")
	case "node", "javascript", "typescript":
		cmd = exec.CommandContext(ctx, "npm", "run", "build")
	case "python":
		cmd = exec.CommandContext(ctx, "python", "-m", "py_compile")
	case "swift":
		cmd = exec.CommandContext(ctx, "swift", "build")
	default:
		return nil, fmt.Errorf("unsupported project type: %s", projectType)
	}

	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]interface{}{
		"status":  "built",
		"project": filepath.Base(projectPath),
		"type":    projectType,
		"output":  string(out),
	})
}

type TestInput struct {
	Path     string `json:"path"`
	Coverage bool   `json:"coverage"`
}

func (w *ProjectWorker) test(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req TestInput
	json.Unmarshal(input, &req)

	projectPath := req.Path
	if projectPath == "" {
		projectPath = w.basePath
	}

	projectType := detectProjectType(projectPath)

	var cmd *exec.Cmd
	switch projectType {
	case "go":
		args := []string{"test", "-v"}
		if req.Coverage {
			args = append(args, "-cover")
		}
		args = append(args, "./...")
		cmd = exec.CommandContext(ctx, "go", args...)
	case "node", "javascript", "typescript":
		cmd = exec.CommandContext(ctx, "npm", "test")
	case "python":
		cmd = exec.CommandContext(ctx, "pytest")
	default:
		return nil, fmt.Errorf("unsupported project type: %s", projectType)
	}

	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]interface{}{
		"status":  "tested",
		"project": filepath.Base(projectPath),
		"output":  string(out),
	})
}

type DepsInput struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

func (w *ProjectWorker) deps(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DepsInput
	json.Unmarshal(input, &req)

	projectPath := req.Path
	if projectPath == "" {
		projectPath = w.basePath
	}

	action := req.Action
	if action == "" {
		action = "list"
	}

	projectType := detectProjectType(projectPath)

	var cmd *exec.Cmd
	switch action {
	case "install":
		switch projectType {
		case "go":
			cmd = exec.CommandContext(ctx, "go", "mod", "download")
		case "node", "javascript", "typescript":
			cmd = exec.CommandContext(ctx, "npm", "install")
		case "python":
			cmd = exec.CommandContext(ctx, "pip", "install", "-r", "requirements.txt")
		}
	case "list":
		switch projectType {
		case "go":
			cmd = exec.CommandContext(ctx, "go", "list", "-m", "all")
		case "node", "javascript", "typescript":
			cmd = exec.CommandContext(ctx, "npm", "ls", "--depth=0")
		case "python":
			cmd = exec.CommandContext(ctx, "pip", "list")
		}
	case "update":
		switch projectType {
		case "go":
			cmd = exec.CommandContext(ctx, "go", "get", "-u", "./...")
		case "node", "javascript", "typescript":
			cmd = exec.CommandContext(ctx, "npm", "update")
		case "python":
			cmd = exec.CommandContext(ctx, "pip", "install", "-U", "-r", "requirements.txt")
		}
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}

	if cmd == nil {
		return nil, fmt.Errorf("unsupported project type: %s", projectType)
	}

	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	return json.Marshal(map[string]interface{}{
		"action":  action,
		"project": filepath.Base(projectPath),
		"output":  string(out),
	})
}

type StructureInput struct {
	Path  string `json:"path"`
	Depth int    `json:"depth"`
}

func (w *ProjectWorker) structure(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req StructureInput
	json.Unmarshal(input, &req)

	projectPath := req.Path
	if projectPath == "" {
		projectPath = w.basePath
	}

	depth := req.Depth
	if depth == 0 {
		depth = 3
	}

	tree, err := getDirTree(projectPath, depth, 0)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]interface{}{
		"project": filepath.Base(projectPath),
		"tree":    tree,
	})
}

func detectProjectType(path string) string {
	files, _ := os.ReadDir(path)
	for _, f := range files {
		name := f.Name()
		switch name {
		case "go.mod":
			return "go"
		case "package.json":
			return "node"
		case "Cargo.toml":
			return "rust"
		case "requirements.txt", "setup.py", "pyproject.toml":
			return "python"
		case "Package.swift":
			return "swift"
		case "pom.xml", "build.gradle":
			return "java"
		}
	}
	return "unknown"
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dest := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, info.Mode())
	})
}

func applyVars(path string, vars map[string]string) {
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		content := string(data)
		for k, v := range vars {
			content = strings.ReplaceAll(content, "{{"+k+"}}", v)
		}
		os.WriteFile(p, []byte(content), info.Mode())
		return nil
	})
}

type TreeNode struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children []TreeNode `json:"children,omitempty"`
}

func getDirTree(path string, maxDepth, currentDepth int) ([]TreeNode, error) {
	if currentDepth >= maxDepth {
		return nil, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var nodes []TreeNode
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		node := TreeNode{
			Name: name,
			Type: "file",
		}
		if e.IsDir() {
			node.Type = "dir"
			children, _ := getDirTree(filepath.Join(path, name), maxDepth, currentDepth+1)
			node.Children = children
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func init() {
	_ = time.Time{}
}
