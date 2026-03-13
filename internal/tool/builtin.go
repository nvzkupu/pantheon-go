package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func Builtins() *Registry {
	r := NewRegistry()
	r.Register(shellTool())
	r.Register(readFileTool())
	r.Register(writeFileTool())
	r.Register(listDirTool())
	r.Register(searchFilesTool())
	return r
}

func shellTool() Tool {
	return NewFunc("shell_exec",
		"Execute a shell command and return stdout/stderr. Times out after 60s.",
		Schema{
			Type: "object",
			Properties: map[string]Schema{
				"command": {Type: "string", Desc: "Shell command to execute"},
				"workdir": {Type: "string", Desc: "Working directory (optional)"},
			},
			Required: []string{"command"},
		},
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct {
				Command string `json:"command"`
				Workdir string `json:"workdir"`
			}](argsJSON)
			if err != nil {
				return "", err
			}
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			shell, flag := shellCmd()
			cmd := exec.CommandContext(ctx, shell, flag, args.Command)
			if args.Workdir != "" {
				cmd.Dir = args.Workdir
			}
			out, err := cmd.CombinedOutput()
			result := strings.TrimSpace(string(out))
			if err != nil {
				return fmt.Sprintf("%s\nexit: %v", result, err), nil
			}
			if result == "" {
				return "(no output)", nil
			}
			return result, nil
		},
	)
}

func readFileTool() Tool {
	return NewFunc("read_file",
		"Read file contents. Fails gracefully if the file doesn't exist.",
		Schema{
			Type:       "object",
			Properties: map[string]Schema{"path": {Type: "string", Desc: "File path"}},
			Required:   []string{"path"},
		},
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct{ Path string `json:"path"` }](argsJSON)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(args.Path)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			return string(data), nil
		},
	)
}

func writeFileTool() Tool {
	return NewFunc("write_file",
		"Write content to a file, creating parent directories as needed.",
		Schema{
			Type: "object",
			Properties: map[string]Schema{
				"path":    {Type: "string", Desc: "File path"},
				"content": {Type: "string", Desc: "Content to write"},
			},
			Required: []string{"path", "content"},
		},
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}](argsJSON)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(args.Path), 0o755); err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
		},
	)
}

func listDirTool() Tool {
	return NewFunc("list_dir",
		"List files and directories at a path. Dirs are suffixed with /.",
		Schema{
			Type:       "object",
			Properties: map[string]Schema{"path": {Type: "string", Desc: "Directory path"}},
			Required:   []string{"path"},
		},
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct{ Path string `json:"path"` }](argsJSON)
			if err != nil {
				return "", err
			}
			entries, err := os.ReadDir(args.Path)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			var b strings.Builder
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				b.WriteString(name)
				b.WriteByte('\n')
			}
			return strings.TrimSpace(b.String()), nil
		},
	)
}

func searchFilesTool() Tool {
	return NewFunc("search_files",
		"Search for files matching a glob pattern. Returns matching paths (max 100).",
		Schema{
			Type: "object",
			Properties: map[string]Schema{
				"pattern": {Type: "string", Desc: "Glob pattern (e.g. '*.go')"},
				"root":    {Type: "string", Desc: "Root directory (default: '.')"},
			},
			Required: []string{"pattern"},
		},
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct {
				Pattern string `json:"pattern"`
				Root    string `json:"root"`
			}](argsJSON)
			if err != nil {
				return "", err
			}
			root := args.Root
			if root == "" {
				root = "."
			}
			var matches []string
			_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if matched, _ := filepath.Match(args.Pattern, filepath.Base(path)); matched {
					matches = append(matches, path)
				}
				if len(matches) >= 100 {
					return filepath.SkipAll
				}
				return nil
			})
			if len(matches) == 0 {
				return "no matches found", nil
			}
			return strings.Join(matches, "\n"), nil
		},
	)
}

func shellCmd() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", "/c"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s, "-c"
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash", "-c"
	}
	return "sh", "-c"
}
