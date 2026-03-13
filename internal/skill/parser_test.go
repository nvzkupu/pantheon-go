package skill

import (
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFM   string
		wantBody string
	}{
		{
			name:     "standard frontmatter",
			input:    "---\nname: test\ndescription: a test\n---\n\n# Body here",
			wantFM:   "name: test\ndescription: a test",
			wantBody: "# Body here",
		},
		{
			name:     "no frontmatter",
			input:    "# Just a markdown file\n\nSome content.",
			wantFM:   "",
			wantBody: "# Just a markdown file\n\nSome content.",
		},
		{
			name:     "empty body",
			input:    "---\nname: test\n---\n",
			wantFM:   "name: test",
			wantBody: "",
		},
		{
			name:     "frontmatter with trailing whitespace",
			input:    "---\nname: test\n---\n\n  # Body  \n",
			wantFM:   "name: test",
			wantBody: "# Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := SplitFrontmatter([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(fm) != tt.wantFM {
				t.Errorf("frontmatter: got %q, want %q", string(fm), tt.wantFM)
			}
			if body != tt.wantBody {
				t.Errorf("body: got %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, s *Skill)
	}{
		{
			name: "standard skill with metadata",
			input: `---
name: athena
description: A strategist skill
license: MIT
metadata:
  persona: Your Devoted Strategist
  model: opus-4
  temperature: 0.5
  max_tokens: 8192
  tools:
    - read_file
    - list_dir
  delegates:
    - kali
---

# Athena

Body content here.`,
			check: func(t *testing.T, s *Skill) {
				assertEqual(t, "name", s.Name, "athena")
				assertEqual(t, "description", s.Description, "A strategist skill")
				assertEqual(t, "license", s.License, "MIT")
				assertEqual(t, "persona", s.Metadata.Persona, "Your Devoted Strategist")
				assertEqual(t, "model", s.Metadata.Model, "opus-4")
				if s.Metadata.Temperature != 0.5 {
					t.Errorf("temperature: got %v, want 0.5", s.Metadata.Temperature)
				}
				if s.Metadata.MaxTokens != 8192 {
					t.Errorf("max_tokens: got %d, want 8192", s.Metadata.MaxTokens)
				}
				if len(s.Metadata.Tools) != 2 {
					t.Errorf("tools count: got %d, want 2", len(s.Metadata.Tools))
				}
				if len(s.Metadata.Delegates) != 1 || s.Metadata.Delegates[0] != "kali" {
					t.Errorf("delegates: got %v, want [kali]", s.Metadata.Delegates)
				}
				if s.Body != "# Athena\n\nBody content here." {
					t.Errorf("body: got %q", s.Body)
				}
			},
		},
		{
			name:    "missing description",
			input:   "---\nname: test\n---\n\nBody",
			wantErr: true,
		},
		{
			name: "name inferred from path",
			input: `---
description: inferred name
---

# Content`,
			check: func(t *testing.T, s *Skill) {
				assertEqual(t, "name", s.Name, "myskill")
			},
		},
		{
			name: "no frontmatter",
			input: `# Just markdown

Some content here.`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/skills/myskill/SKILL.md"
			s, err := ParseBytes([]byte(tt.input), path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
