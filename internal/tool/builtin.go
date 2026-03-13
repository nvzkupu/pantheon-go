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

const maxReadSize = 10 << 20 // 10MB

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
		"Execute a shell command in the system shell and return combined stdout/stderr output. "+
			"Use this tool to run build commands, install packages, query system state, or perform any operation available via the command line. "+
			"The command times out after 60 seconds; long-running processes will be killed and an error returned. "+
			"On Windows the command runs via cmd.exe; on Unix it uses the user's default shell. "+
			"Returns '(no output)' when the command succeeds but produces no output.",
		StrictSchema(map[string]Schema{
			"command": {Type: "string", Desc: "The shell command to execute, e.g. 'go build ./...' or 'ls -la'"},
			"workdir": {Type: "string", Desc: "Working directory for the command. Pass an empty string to use the current directory"},
		}, []string{"command", "workdir"}),
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
		"Read the full contents of a file and return it as a UTF-8 string. "+
			"Use this tool when you need to inspect source code, configuration files, or any text file. "+
			"Returns a descriptive error message if the file does not exist or cannot be read. "+
			"This tool does not support binary files; use shell_exec for binary operations.",
		StrictSchema(map[string]Schema{
			"path": {Type: "string", Desc: "Absolute or relative file path to read, e.g. 'src/main.go'"},
		}, []string{"path"}),
		func(ctx context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct{ Path string `json:"path"` }](argsJSON)
			if err != nil {
				return "", err
			}
			info, err := os.Stat(args.Path)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			if info.Size() > maxReadSize {
				return fmt.Sprintf("error: file is too large (%d bytes, max %d bytes)", info.Size(), maxReadSize), nil
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
		"Write text content to a file, creating any missing parent directories automatically. "+
			"Use this tool to create new files or overwrite existing ones with the provided content. "+
			"The file is written with UTF-8 encoding and 0644 permissions. "+
			"Returns a confirmation with the number of bytes written, or an error if the write fails.",
		StrictSchema(map[string]Schema{
			"path":    {Type: "string", Desc: "Absolute or relative file path to write, e.g. 'output/result.json'"},
			"content": {Type: "string", Desc: "The full text content to write to the file"},
		}, []string{"path", "content"}),
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
		"List all files and subdirectories in a directory, one entry per line. "+
			"Directory entries are suffixed with '/' to distinguish them from files. "+
			"Use this tool to explore project structure or verify that expected files exist. "+
			"Returns an error if the path does not exist or is not a directory.",
		StrictSchema(map[string]Schema{
			"path": {Type: "string", Desc: "Absolute or relative path to the directory to list, e.g. 'src/'"},
		}, []string{"path"}),
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
		"Recursively search for files whose names match a glob pattern, starting from a root directory. "+
			"Returns matching file paths (one per line), capped at 100 results. "+
			"The pattern is matched against the file basename only, not the full path — use '*.go' not '**/*.go'. "+
			"Returns 'no matches found' if no files match the pattern.",
		StrictSchema(map[string]Schema{
			"pattern": {Type: "string", Desc: "Glob pattern matched against file names, e.g. '*.go', '*.test.js', 'Makefile'"},
			"root":    {Type: "string", Desc: "Root directory to search from. Pass an empty string to use the current directory"},
		}, []string{"pattern", "root"}),
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
			_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
