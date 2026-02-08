package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSkillsAndFilter(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))

	alphaPath := filepath.Join(skillsDir, "alpha.md")
	require.NoError(t, os.WriteFile(alphaPath, []byte("# Alpha\nDo alpha things."), 0644))

	betaDir := filepath.Join(skillsDir, "beta")
	require.NoError(t, os.MkdirAll(betaDir, 0755))
	betaPath := filepath.Join(betaDir, "SKILL.md")
	require.NoError(t, os.WriteFile(betaPath, []byte("# Beta\nDo beta things."), 0644))

	skills, err := loadSkills(skillsDir)
	require.NoError(t, err)
	require.Len(t, skills, 2)

	filtered := filterSkillsByRefs(skills, "please use @skill:beta now")
	require.Len(t, filtered, 1)
	assert.Equal(t, "beta", filtered[0].Name)

	all := filterSkillsByRefs(skills, "use @skill:all")
	require.Len(t, all, 2)

	none := filterSkillsByRefs(skills, "use @skill:none")
	assert.Len(t, none, 0)
}
