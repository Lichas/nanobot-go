package skills

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// AnthricSkillsRepo 官方 skills 仓库
	AnthricSkillsRepo = "anthropics/skills"
	// PlaywrightSkillsRepo Microsoft Playwright CLI skills 仓库
	PlaywrightSkillsRepo = "microsoft/playwright-cli"
	// DefaultSkillsBranch 默认分支
	DefaultSkillsBranch = "main"
	// SkillsInstallMarker 安装标记文件
	SkillsInstallMarker = ".official_skills_installed"
)

// 镜像源列表（按优先级排序）
var mirrorSources = []struct {
	name string
	url  string
}{
	{"GitHub", "https://github.com/%s/archive/refs/heads/%s.zip"},
	{"FastGit", "https://hub.fastgit.xyz/%s/archive/refs/heads/%s.zip"},
	{"GhProxy", "https://ghproxy.com/https://github.com/%s/archive/refs/heads/%s.zip"},
	{"GhProxy-CN", "https://ghproxy.cn/https://github.com/%s/archive/refs/heads/%s.zip"},
	{"Moeyy", "https://github.moeyy.xyz/https://github.com/%s/archive/refs/heads/%s.zip"},
}

// Installer 负责安装官方 skills
type Installer struct {
	workspace  string
	httpClient *http.Client
}

type InstallResult struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
	Version  string `json:"version,omitempty"`
	Source   string `json:"source,omitempty"`
	Registry string `json:"registry,omitempty"`
}

// NewInstaller 创建 skills 安装器
func NewInstaller(workspace string) *Installer {
	return &Installer{
		workspace: workspace,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment, // 支持系统代理
			},
		},
	}
}

// IsFirstRun 检查是否是首次运行（skills 目录为空或没有官方 skills）
func (i *Installer) IsFirstRun() bool {
	skillsDir := filepath.Join(i.workspace, "skills")

	// 检查标记文件
	markerPath := filepath.Join(skillsDir, SkillsInstallMarker)
	if _, err := os.Stat(markerPath); err == nil {
		return false
	}

	// 检查 skills 目录是否存在且有内容
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		// 目录不存在，需要安装
		return true
	}

	// 如果 skills 目录为空或只有 README.md，视为首次运行
	for _, entry := range entries {
		name := entry.Name()
		if name != "README.md" && !strings.HasPrefix(name, ".") {
			// 有非 README 文件，说明用户已添加自己的 skills
			return false
		}
	}

	return true
}

// OfficialRepo 定义官方技能仓库
type OfficialRepo struct {
	Name    string
	Repo    string
	SubPath string // 可选：指定子路径，如 "skills/steipete/weather"
}

// officialRepos 官方技能仓库列表（按安装顺序）
var officialRepos = []OfficialRepo{
	{Name: "Anthropics", Repo: AnthricSkillsRepo},
	{Name: "Playwright", Repo: PlaywrightSkillsRepo},
	// 按需安装特定子路径示例：
	// {Name: "OpenClawWeather", Repo: "openclaw/skills", SubPath: "skills/steipete/weather"},
}

// InstallOfficialSkills 从 GitHub 或镜像下载并安装所有官方 skills
// 支持自动 fallback 到可用镜像
func (i *Installer) InstallOfficialSkills() error {
	skillsDir := filepath.Join(i.workspace, "skills")

	// 确保 skills 目录存在
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	installedRepos := []string{}
	totalInstalled := 0

	// 遍历所有官方仓库
	for _, repo := range officialRepos {
		fmt.Printf("\n📦 Installing %s skills...\n", repo.Name)
		count, err := i.InstallRepoSkills(repo)
		if err != nil {
			// 检查是否是网络错误
			if _, ok := err.(*NetworkError); ok {
				fmt.Printf("  ⚠ Network issue for %s, skipping...\n", repo.Name)
				continue
			}
			// 其他错误（如解压失败）记录但不中断
			fmt.Printf("  ⚠ Failed to install %s: %v\n", repo.Name, err)
			continue
		}
		fmt.Printf("  ✓ Installed %d skills from %s\n", count, repo.Name)
		installedRepos = append(installedRepos, repo.Repo)
		totalInstalled += count
	}

	if totalInstalled == 0 {
		return &NetworkError{
			Message: "failed to download skills from all mirrors and repos",
		}
	}

	// 创建安装标记
	markerPath := filepath.Join(skillsDir, SkillsInstallMarker)
	var markerContent strings.Builder
	markerContent.WriteString(fmt.Sprintf("Official skills installed at: %s\n", time.Now().Format(time.RFC3339)))
	markerContent.WriteString(fmt.Sprintf("Total skills installed: %d\n", totalInstalled))
	markerContent.WriteString("Sources:\n")
	for _, repo := range installedRepos {
		markerContent.WriteString(fmt.Sprintf("  - https://github.com/%s\n", repo))
	}
	if err := os.WriteFile(markerPath, []byte(markerContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to create install marker: %w", err)
	}

	fmt.Printf("\n✓ Official skills installed successfully! Total: %d\n", totalInstalled)
	return nil
}

// InstallRepoSkills 安装单个仓库的技能（公开方法）
// 返回安装的文件数量和可能的错误
func (i *Installer) InstallRepoSkills(repo OfficialRepo) (int, error) {
	skillsDir := filepath.Join(i.workspace, "skills")

	// 如果指定了子路径，使用子目录安装方式
	if repo.SubPath != "" {
		return i.installSubPathSkills(repo, skillsDir)
	}

	// 完整仓库安装（原有逻辑）
	zipPath := filepath.Join(i.workspace, fmt.Sprintf(".tmp_skills_%s.zip", strings.ReplaceAll(repo.Repo, "/", "_")))
	defer os.Remove(zipPath)

	var lastErr error
	for _, source := range mirrorSources {
		zipURL := fmt.Sprintf(source.url, repo.Repo, DefaultSkillsBranch)
		fmt.Printf("  Trying %s...\n", source.name)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := i.downloadFileWithContext(ctx, zipURL, zipPath)
		cancel()

		if err == nil {
			fmt.Printf("    ✓ Downloaded from %s\n", source.name)
			break
		}

		lastErr = err
		// 检查是否是网络连接问题
		if isNetworkError(err) {
			fmt.Printf("    ✗ %s unavailable\n", source.name)
			continue
		}
		// 其他错误直接返回
		return 0, fmt.Errorf("download failed from %s: %w", source.name, err)
	}

	// 检查是否下载成功
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return 0, &NetworkError{
			Message: fmt.Sprintf("failed to download %s from all mirrors", repo.Name),
			Cause:   lastErr,
		}
	}

	// 解压并安装
	count, err := i.extractSkills(zipPath, skillsDir)
	if err != nil {
		return 0, fmt.Errorf("failed to extract skills: %w", err)
	}

	return count, nil
}

// installSubPathSkills 安装仓库中特定子路径的技能
// 使用 GitHub Contents API 逐个下载文件
func (i *Installer) installSubPathSkills(repo OfficialRepo, targetDir string) (int, error) {
	// 提取 owner 和 repo 名称
	parts := strings.Split(repo.Repo, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid repo format: %s", repo.Repo)
	}
	owner, repoName := parts[0], parts[1]

	// 构建 API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repoName, repo.SubPath, DefaultSkillsBranch)

	fmt.Printf("  Fetching from GitHub API: %s...\n", repo.SubPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 获取目录内容
	contents, err := i.fetchGitHubContents(ctx, apiURL)
	if err != nil {
		// API 失败时尝试用镜像下载整个仓库然后提取
		fmt.Printf("  API failed (%v), falling back to zip extract...\n", err)
		return i.installSubPathFromZip(repo, targetDir)
	}

	// 下载目录中的文件
	installedCount := 0
	for _, item := range contents {
		if item.Type == "file" && strings.HasSuffix(item.Name, ".md") {
			targetPath := filepath.Join(targetDir, item.Name)
			if err := i.downloadSkillFile(ctx, item.DownloadURL, targetPath); err != nil {
				fmt.Printf("    ⚠ Failed to download %s: %v\n", item.Name, err)
				continue
			}
			fmt.Printf("    ✓ Downloaded %s\n", item.Name)
			installedCount++
		}
	}

	return installedCount, nil
}

// githubContentItem 表示 GitHub API 返回的单个内容项
type githubContentItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
	HTMLURL     string `json:"html_url"`
}

// fetchGitHubContents 调用 GitHub API 获取目录内容
func (i *Installer) fetchGitHubContents(ctx context.Context, apiURL string) ([]githubContentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "maxclaw-skills-installer/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %s", resp.Status)
	}

	var contents []githubContentItem
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return nil, err
	}

	return contents, nil
}

// downloadSkillFile 下载单个技能文件
func (i *Installer) downloadSkillFile(ctx context.Context, url, targetPath string) error {
	return i.downloadFileWithContext(ctx, url, targetPath)
}

// installSubPathFromZip 作为备选方案：下载整个仓库 ZIP，然后只提取子路径
func (i *Installer) installSubPathFromZip(repo OfficialRepo, targetDir string) (int, error) {
	zipPath := filepath.Join(i.workspace, fmt.Sprintf(".tmp_skills_%s.zip", strings.ReplaceAll(repo.Repo, "/", "_")))
	defer os.Remove(zipPath)

	var lastErr error
	for _, source := range mirrorSources {
		zipURL := fmt.Sprintf(source.url, repo.Repo, DefaultSkillsBranch)
		fmt.Printf("  Trying %s...\n", source.name)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := i.downloadFileWithContext(ctx, zipURL, zipPath)
		cancel()

		if err == nil {
			fmt.Printf("    ✓ Downloaded from %s\n", source.name)
			break
		}

		lastErr = err
		if isNetworkError(err) {
			continue
		}
		return 0, err
	}

	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return 0, &NetworkError{
			Message: fmt.Sprintf("failed to download %s from all mirrors", repo.Repo),
			Cause:   lastErr,
		}
	}

	// 提取子路径
	return i.extractSubPath(zipPath, targetDir, repo.SubPath)
}

// extractSubPath 从 ZIP 中提取特定子路径的文件
func (i *Installer) extractSubPath(zipPath, targetDir, subPath string) (int, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	// ZIP 中的路径前缀通常是 "repo-main/"
	var repoPrefix string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			// 找到第一个目录作为前缀
			parts := strings.Split(f.Name, "/")
			if len(parts) > 0 && repoPrefix == "" {
				repoPrefix = parts[0] + "/"
				break
			}
		}
	}

	fullPrefix := repoPrefix + subPath + "/"
	installedCount := 0

	for _, f := range r.File {
		// 只匹配子路径下的 .md 文件
		if !strings.HasPrefix(f.Name, fullPrefix) {
			continue
		}
		if !strings.HasSuffix(f.Name, ".md") {
			continue
		}

		relPath := strings.TrimPrefix(f.Name, fullPrefix)
		if strings.Contains(relPath, "/") {
			// 跳过后代目录中的文件，只取直接子文件
			continue
		}

		targetPath := filepath.Join(targetDir, relPath)

		rc, err := f.Open()
		if err != nil {
			continue
		}

		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			continue
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()

		if err == nil {
			installedCount++
		}
	}

	return installedCount, nil
}

// NetworkError 网络错误类型
type NetworkError struct {
	Message string
	Cause   error
}

func (e *NetworkError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *NetworkError) Unwrap() error {
	return e.Cause
}

// IsNetworkError 检查是否是网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查 URL 错误
	if urlErr, ok := err.(*url.Error); ok {
		// 超时或临时错误
		if urlErr.Timeout() || urlErr.Temporary() {
			return true
		}
	}

	// 检查错误消息
	errStr := err.Error()
	networkKeywords := []string{
		"connection refused",
		"no such host",
		"timeout",
		"i/o timeout",
		"temporary failure",
		"connection reset",
		"EOF",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// downloadFileWithContext 带上下文的文件下载
func (i *Installer) downloadFileWithContext(ctx context.Context, url, filepath string) error {
	resp, err := i.getWithRetry(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return formatHTTPStatusError(resp)
	}

	// 创建目标文件
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (i *Installer) downloadBytesWithContext(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := i.getWithRetry(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, formatHTTPStatusError(resp)
	}

	return io.ReadAll(resp.Body)
}

func (i *Installer) getWithRetry(ctx context.Context, rawURL string) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent", "maxclaw-skills-installer/1.0")

		resp, err := i.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt == 3 || !isNetworkError(err) {
				return nil, err
			}
			if err := waitForRetry(ctx, retryDelay(nil, attempt)); err != nil {
				return nil, err
			}
			continue
		}

		if !shouldRetryHTTPStatus(resp.StatusCode) {
			return resp, nil
		}

		lastErr = formatHTTPStatusError(resp)
		resp.Body.Close()
		if attempt == 3 {
			return nil, lastErr
		}
		if err := waitForRetry(ctx, retryDelay(resp.Header, attempt)); err != nil {
			return nil, err
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("request failed: %s", rawURL)
}

func shouldRetryHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func retryDelay(header http.Header, attempt int) time.Duration {
	if header != nil {
		if parsed, ok := parseRetryAfter(header.Get("Retry-After")); ok {
			return parsed
		}
	}

	delay := 500 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

func parseRetryAfter(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return time.Duration(seconds) * time.Second, true
	}

	if ts, err := http.ParseTime(raw); err == nil {
		delay := time.Until(ts)
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}

	return 0, false
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func formatHTTPStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	return fmt.Errorf("bad status: %s: %s", resp.Status, bodyText)
}

// extractSkills 解压 zip 文件中的 skills 到目标目录
// 返回安装的文件数量
func (i *Installer) extractSkills(zipPath, targetDir string) (int, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	// 找到 skills 目录的前缀
	skillsPrefix := ""
	for _, f := range r.File {
		if strings.Contains(f.Name, "/skills/") {
			parts := strings.Split(f.Name, "/")
			for i, part := range parts {
				if part == "skills" && i > 0 {
					skillsPrefix = strings.Join(parts[:i+1], "/") + "/"
					break
				}
			}
			break
		}
	}

	if skillsPrefix == "" {
		return 0, fmt.Errorf("could not find skills directory in archive")
	}

	installedCount := 0
	for _, f := range r.File {
		// 只处理 skills 目录下的文件
		if !strings.HasPrefix(f.Name, skillsPrefix) {
			continue
		}

		// 跳过根目录和特殊文件
		relPath := strings.TrimPrefix(f.Name, skillsPrefix)
		if relPath == "" || strings.HasPrefix(relPath, ".") {
			continue
		}

		targetPath := filepath.Join(targetDir, relPath)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, f.Mode()); err != nil {
				return 0, err
			}
			continue
		}

		// 创建文件
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return 0, err
		}

		rc, err := f.Open()
		if err != nil {
			return 0, err
		}

		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return 0, err
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()

		if err != nil {
			return 0, err
		}

		installedCount++
	}

	return installedCount, nil
}

func (i *Installer) extractArchive(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		targetPath := filepath.Join(targetDir, filepath.Clean(f.Name))
		rel, err := filepath.Rel(targetDir, targetPath)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("invalid archive path: %s", f.Name)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

// InstallIfNeeded 如果需要则安装官方 skills（用于自动检测）
func (i *Installer) InstallIfNeeded() error {
	if !i.IsFirstRun() {
		return nil
	}

	return i.InstallOfficialSkills()
}

// ListInstalledSkills 列出已安装的官方 skills
func (i *Installer) ListInstalledSkills() ([]string, error) {
	skillsDir := filepath.Join(i.workspace, "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var skillsList []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "README.md" || name == SkillsInstallMarker || strings.HasPrefix(name, ".") {
			continue
		}

		// 检查是否是有效的 skill（包含 SKILL.md 或 .md 文件）
		if entry.IsDir() {
			skillFile := filepath.Join(skillsDir, name, "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				skillsList = append(skillsList, name)
				continue
			}
			// 也检查目录下是否有 .md 文件
			if hasMarkdownFiles(filepath.Join(skillsDir, name)) {
				skillsList = append(skillsList, name)
			}
		} else if strings.HasSuffix(name, ".md") {
			skillName := strings.TrimSuffix(name, ".md")
			skillsList = append(skillsList, skillName)
		}
	}

	return skillsList, nil
}

// hasMarkdownFiles 检查目录下是否有 markdown 文件
func hasMarkdownFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			return true
		}
	}
	return false
}

// GetInstallHelpMessage 获取安装帮助信息（网络失败时显示）
func GetInstallHelpMessage() string {
	return `
Skills installation failed due to network issues.

Options:
  1. Configure proxy and retry:
     export HTTPS_PROXY=http://127.0.0.1:7890
     maxclaw skills install --official

  2. Manual download:
     - Anthropics: https://github.com/anthropics/skills/archive/refs/heads/main.zip
     - Playwright: https://github.com/microsoft/playwright-cli/archive/refs/heads/main.zip
     - Extract the 'skills' folder to: ~/.maxclaw/workspace/skills/

  3. Use a mirror:
     The installer already tried multiple mirrors (FastGit, GhProxy, etc.)
     If all failed, you may need a system-wide VPN/proxy.

  4. Skip for now:
     maxclaw works without official skills. You can add your own skills
     to ~/.maxclaw/workspace/skills/ later.
`
}

// GitHubSource 表示解析后的 GitHub 源信息
type GitHubSource struct {
	Owner  string
	Repo   string
	Branch string
	Path   string
	Type   string // "file", "dir", "repo"
}

// InstallFromGitHub 从 GitHub 智能安装技能
func (i *Installer) InstallFromGitHub(source GitHubSource) (int, error) {
	targetDir := filepath.Join(i.workspace, "skills")

	switch source.Type {
	case "file":
		return i.installSingleFile(source, targetDir)
	case "dir":
		return i.installSubPath(source, targetDir)
	case "repo":
		repo := OfficialRepo{
			Name: fmt.Sprintf("%s/%s", source.Owner, source.Repo),
			Repo: fmt.Sprintf("%s/%s", source.Owner, source.Repo),
		}
		return i.InstallRepoSkills(repo)
	default:
		return 0, fmt.Errorf("unsupported source type: %s", source.Type)
	}
}

// installSingleFile 安装单个技能文件
func (i *Installer) installSingleFile(source GitHubSource, targetDir string) (int, error) {
	// 构建 raw 内容 URL
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		source.Owner, source.Repo, source.Branch, source.Path)

	// 确定目标文件名
	fileName := filepath.Base(source.Path)
	if !strings.HasSuffix(fileName, ".md") {
		fileName += ".md"
	}

	targetPath := filepath.Join(targetDir, fileName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := i.downloadFileWithContext(ctx, rawURL, targetPath); err != nil {
		return 0, fmt.Errorf("failed to download file: %w", err)
	}

	return 1, nil
}

// installSubPath 安装子路径下的所有技能文件
func (i *Installer) installSubPath(source GitHubSource, targetDir string) (int, error) {
	repo := OfficialRepo{
		Name:    fmt.Sprintf("%s/%s", source.Owner, source.Repo),
		Repo:    fmt.Sprintf("%s/%s", source.Owner, source.Repo),
		SubPath: source.Path,
	}
	return i.installSubPathSkills(repo, targetDir)
}

// InstallSingleFile 公开方法：从 GitHub 安装单个技能文件
func (i *Installer) InstallSingleFile(source GitHubSource) error {
	targetDir := filepath.Join(i.workspace, "skills")
	_, err := i.installSingleFile(source, targetDir)
	return err
}

// InstallSubPath 公开方法：从 GitHub 安装子路径下的所有技能
func (i *Installer) InstallSubPath(source GitHubSource) error {
	targetDir := filepath.Join(i.workspace, "skills")
	_, err := i.installSubPath(source, targetDir)
	return err
}

type clawHubSkillDetailResponse struct {
	Skill struct {
		Slug        string            `json:"slug"`
		DisplayName string            `json:"displayName"`
		Tags        map[string]string `json:"tags"`
	} `json:"skill"`
	LatestVersion struct {
		Version string `json:"version"`
	} `json:"latestVersion"`
}

func (i *Installer) InstallFromClawHub(source ClawHubSource) (*InstallResult, error) {
	if strings.TrimSpace(source.Registry) == "" {
		source.Registry = DefaultClawHubRegistry
	}
	if strings.TrimSpace(source.Slug) == "" {
		return nil, fmt.Errorf("clawhub slug is required")
	}

	metaURL := fmt.Sprintf("%s/api/v1/skills/%s", strings.TrimRight(source.Registry, "/"), url.PathEscape(source.Slug))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payload, err := i.downloadBytesWithContext(ctx, metaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clawhub metadata: %w", err)
	}

	var meta clawHubSkillDetailResponse
	if err := json.Unmarshal(payload, &meta); err != nil {
		return nil, fmt.Errorf("failed to decode clawhub metadata: %w", err)
	}

	version := strings.TrimSpace(source.Version)
	if version == "" {
		version = strings.TrimSpace(meta.LatestVersion.Version)
	}
	tag := strings.TrimSpace(source.Tag)
	if version == "" && tag == "" {
		tag = strings.TrimSpace(meta.Skill.Tags["latest"])
	}
	if version == "" && tag == "" {
		return nil, fmt.Errorf("clawhub skill %q does not expose a latest version", source.Slug)
	}

	targetDir := filepath.Join(i.workspace, "skills", source.Slug)
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, fmt.Errorf("failed to reset skill directory: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skill directory: %w", err)
	}

	query := url.Values{}
	query.Set("slug", source.Slug)
	if version != "" {
		query.Set("version", version)
	} else {
		query.Set("tag", tag)
	}

	zipURL := fmt.Sprintf("%s/api/v1/download?%s", strings.TrimRight(source.Registry, "/"), query.Encode())
	zipPath := filepath.Join(i.workspace, fmt.Sprintf(".tmp_clawhub_%s.zip", source.Slug))
	defer os.Remove(zipPath)

	if err := i.downloadFileWithContext(ctx, zipURL, zipPath); err != nil {
		return nil, fmt.Errorf("failed to download clawhub archive: %w", err)
	}

	if err := i.extractArchive(zipPath, targetDir); err != nil {
		return nil, fmt.Errorf("failed to extract clawhub archive: %w", err)
	}

	resolvedVersion := version
	if resolvedVersion == "" {
		resolvedVersion = tag
	}

	return &InstallResult{
		Name:     source.Slug,
		Type:     "clawhub",
		Location: targetDir,
		Version:  resolvedVersion,
		Source:   source.Slug,
		Registry: strings.TrimRight(source.Registry, "/"),
	}, nil
}
