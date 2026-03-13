package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SkillsDir() string {
	if dir := os.Getenv("SKILLS_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("AGENTS_DIR"); dir != "" {
		return dir
	}
	candidates := []string{
		filepath.Join(".", ".agents", "skills"),
		filepath.Join(".", ".cursor", "skills"),
		filepath.Join(".", ".claude", "skills"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return candidates[0]
}

func GatewayURL() string {
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := os.Getenv("NVIDIA_GATEWAY_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://integrate.api.nvidia.com/v1"
}

func APIKey() string {
	if k := os.Getenv("API_KEY"); k != "" {
		return k
	}
	return os.Getenv("NVIDIA_API_KEY")
}

func MemoryDir() string {
	if d := os.Getenv("MEMORY_DIR"); d != "" {
		return d
	}
	return ".memory"
}

func Verbose() bool {
	v := os.Getenv("VERBOSE")
	return v == "1" || v == "true"
}

// LoadEnvFile reads a .env file and sets any variables not already in the environment.
func LoadEnvFile(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env"}
		if exe, err := os.Executable(); err == nil {
			paths = append([]string{filepath.Join(filepath.Dir(exe), "..", ".env")}, paths...)
		}
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			v = strings.Trim(v, "\"'")
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: reading %s: %v\n", p, err)
		}
		return
	}
}
