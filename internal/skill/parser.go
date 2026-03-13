package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	License     string   `yaml:"license"`
	Body        string   `yaml:"-"`
	Path        string   `yaml:"-"`
	Metadata    Metadata `yaml:"metadata"`
}

type Metadata struct {
	Persona       string   `yaml:"persona"`
	Model         string   `yaml:"model"`
	Temperature   float64  `yaml:"temperature"`
	MaxTokens     int      `yaml:"max_tokens"`
	MaxIterations int      `yaml:"max_iterations"`
	Tools         []string `yaml:"tools"`
	Delegates     []string `yaml:"delegates"`
}

// Parse reads a SKILL.md file and returns a parsed Skill.
func Parse(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill %s: %w", path, err)
	}
	return ParseBytes(data, path)
}

// ParseBytes parses SKILL.md content from raw bytes. The path argument is used
// only for error messages and is stored on the returned Skill.
func ParseBytes(data []byte, path string) (*Skill, error) {
	fm, body, err := SplitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parse skill %s: %w", path, err)
	}

	var s Skill
	if len(fm) > 0 {
		if err := yaml.Unmarshal(fm, &s); err != nil {
			return nil, fmt.Errorf("parse skill frontmatter %s: %w", path, err)
		}
	}

	if s.Description == "" {
		return nil, fmt.Errorf("skill %s: missing required field 'description'", path)
	}

	if s.Name == "" {
		s.Name = filepath.Base(filepath.Dir(path))
	}

	s.Body = body
	s.Path = path
	return &s, nil
}

// Discover walks a skills directory and returns all valid skills found.
// It looks for SKILL.md files in immediate subdirectories: dir/*/SKILL.md.
func Discover(dir string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	var skills []*Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		s, err := Parse(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", p, err)
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// DiscoverMap is like Discover but returns a map keyed by skill name.
func DiscoverMap(dir string) (map[string]*Skill, error) {
	skills, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*Skill, len(skills))
	for _, s := range skills {
		m[s.Name] = s
	}
	return m, nil
}

// SplitFrontmatter separates YAML frontmatter (between --- delimiters)
// from the markdown body.
func SplitFrontmatter(data []byte) (frontmatter []byte, body string, err error) {
	content := bytes.TrimSpace(data)
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, string(content), nil
	}

	rest := content[3:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return nil, string(content), nil
	}

	fm := bytes.TrimSpace(rest[:idx])
	body = strings.TrimSpace(string(rest[idx+4:]))
	return fm, body, nil
}
