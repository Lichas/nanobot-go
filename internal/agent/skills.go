package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxSkillRunes       = 12000
	maxSkillsTotalRunes = 60000
)

var skillRefPattern = regexp.MustCompile(`(?i)@skill:([a-z0-9_.-]+)`)

type skill struct {
	Name        string
	DisplayName string
	Path        string
	Body        string
}

func loadSkills(skillsDir string) ([]skill, error) {
	info, err := os.Stat(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path is not a directory: %s", skillsDir)
	}

	var skills []skill
	err = filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		name := inferSkillName(path)
		title, body := extractTitleAndBody(string(content))
		if title == "" {
			title = name
		}

		body = strings.TrimSpace(body)
		if body == "" {
			body = "(empty skill)"
		}

		body = truncateRunes(body, maxSkillRunes, "\n\n... (skill truncated)")

		skills = append(skills, skill{
			Name:        strings.ToLower(name),
			DisplayName: title,
			Path:        path,
			Body:        body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

func inferSkillName(path string) string {
	base := filepath.Base(path)
	if strings.EqualFold(base, "SKILL.md") {
		return filepath.Base(filepath.Dir(path))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func extractTitleAndBody(content string) (string, string) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(trim, "# "))
			body := strings.Join(lines[i+1:], "\n")
			return title, body
		}
		break
	}
	return "", content
}

func truncateRunes(s string, limit int, suffix string) string {
	if limit <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	if limit > len(runes) {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + suffix
}

func filterSkillsByRefs(skills []skill, message string) []skill {
	matches := skillRefPattern.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return skills
	}

	wanted := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		ref := strings.ToLower(strings.TrimSpace(match[1]))
		if ref == "" {
			continue
		}
		if ref == "all" {
			return skills
		}
		if ref == "none" {
			return nil
		}
		wanted[ref] = struct{}{}
	}

	if len(wanted) == 0 {
		return skills
	}

	filtered := make([]skill, 0, len(wanted))
	for _, s := range skills {
		if _, ok := wanted[strings.ToLower(s.Name)]; ok {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

func (b *ContextBuilder) buildSkillsSection(currentMessage string) string {
	skillsDir := filepath.Join(b.workspace, "skills")
	skills, err := loadSkills(skillsDir)
	if err != nil || len(skills) == 0 {
		return ""
	}

	skills = filterSkillsByRefs(skills, currentMessage)
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Skills\n")

	used := 0
	for _, s := range skills {
		section := fmt.Sprintf("### %s\n%s\n\n", s.DisplayName, s.Body)
		sectionRunes := utf8.RuneCountInString(section)
		if used+sectionRunes > maxSkillsTotalRunes {
			sb.WriteString("... (skills truncated)\n")
			break
		}
		sb.WriteString(section)
		used += sectionRunes
	}

	return strings.TrimSpace(sb.String())
}
