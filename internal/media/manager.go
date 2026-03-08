package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Lichas/maxclaw/internal/bus"
)

type ResolvedMedia struct {
	LocalPath string
	Filename  string
	MimeType  string
}

type Resolver interface {
	Stage(ctx context.Context, attachment *bus.MediaAttachment) (*ResolvedMedia, error)
}

type Manager struct {
	rootDir   string
	resolvers map[string]Resolver
}

func NewManager(rootDir string) *Manager {
	return &Manager{
		rootDir:   rootDir,
		resolvers: make(map[string]Resolver),
	}
}

func (m *Manager) Register(channel string, resolver Resolver) {
	if resolver == nil {
		return
	}
	m.resolvers[strings.TrimSpace(channel)] = resolver
}

func (m *Manager) StageInbound(ctx context.Context, channel string, attachment *bus.MediaAttachment) (*bus.MediaAttachment, error) {
	if attachment == nil {
		return nil, nil
	}

	resolver, ok := m.resolvers[strings.TrimSpace(channel)]
	if !ok {
		return attachment, nil
	}

	resolved, err := resolver.Stage(ctx, attachment)
	if err != nil {
		return attachment, err
	}
	if resolved == nil {
		return attachment, nil
	}

	cloned := *attachment
	if resolved.LocalPath != "" {
		cloned.LocalPath = resolved.LocalPath
	}
	if resolved.Filename != "" {
		cloned.Filename = resolved.Filename
	}
	if resolved.MimeType != "" {
		cloned.MimeType = resolved.MimeType
	}
	return &cloned, nil
}

type URLResolver struct {
	rootDir    string
	channel    string
	httpClient *http.Client
}

func NewQQResolver(rootDir string, client *http.Client) *URLResolver {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &URLResolver{
		rootDir:    rootDir,
		channel:    "qq",
		httpClient: client,
	}
}

func (r *URLResolver) Stage(ctx context.Context, attachment *bus.MediaAttachment) (*ResolvedMedia, error) {
	sourceURL := strings.TrimSpace(attachment.URL)
	if sourceURL == "" {
		return nil, fmt.Errorf("%s media URL is empty", r.channel)
	}
	return stageRemoteMedia(ctx, r.rootDir, r.channel, sourceURL, attachment.Filename, attachment.MimeType, r.httpClient)
}

type TelegramResolver struct {
	rootDir    string
	token      string
	apiBaseURL string
	httpClient *http.Client
}

func NewTelegramResolver(rootDir, token, proxy string) *TelegramResolver {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if proxy != "" {
		if parsed, err := url.Parse(strings.TrimSpace(proxy)); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &TelegramResolver{
		rootDir:    rootDir,
		token:      strings.TrimSpace(token),
		apiBaseURL: "https://api.telegram.org",
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (r *TelegramResolver) Stage(ctx context.Context, attachment *bus.MediaAttachment) (*ResolvedMedia, error) {
	if strings.TrimSpace(attachment.FileID) == "" {
		if strings.TrimSpace(attachment.URL) == "" {
			return nil, fmt.Errorf("telegram media file_id is empty")
		}
		return stageRemoteMedia(ctx, r.rootDir, "telegram", attachment.URL, attachment.Filename, attachment.MimeType, r.httpClient)
	}

	filePath, err := r.getFilePath(ctx, attachment.FileID)
	if err != nil {
		return nil, err
	}

	sourceURL := strings.TrimRight(r.apiBaseURL, "/") + "/file/bot" + r.token + "/" + strings.TrimLeft(filePath, "/")
	filename := strings.TrimSpace(attachment.Filename)
	if filename == "" {
		filename = filepath.Base(filePath)
	}
	return stageRemoteMedia(ctx, r.rootDir, "telegram", sourceURL, filename, attachment.MimeType, r.httpClient)
}

func (r *TelegramResolver) getFilePath(ctx context.Context, fileID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.apiBaseURL, "/")+"/bot"+r.token+"/getFile?file_id="+url.QueryEscape(fileID), nil)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("telegram getFile failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.OK || strings.TrimSpace(payload.Result.FilePath) == "" {
		return "", fmt.Errorf("telegram getFile returned empty file path")
	}
	return payload.Result.FilePath, nil
}

func stageRemoteMedia(ctx context.Context, rootDir, channel, sourceURL, filenameHint, mimeType string, client *http.Client) (*ResolvedMedia, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download media failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	resolvedMime := strings.TrimSpace(mimeType)
	if resolvedMime == "" {
		resolvedMime = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}

	dir := filepath.Join(rootDir, channel, time.Now().Format("20060102"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	ext := guessExtension(filenameHint, resolvedMime, sourceURL)
	hashInput := sourceURL + "|" + filenameHint + "|" + time.Now().UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(hashInput))
	name := hex.EncodeToString(sum[:12]) + ext
	targetPath := filepath.Join(dir, name)

	file, err := os.Create(targetPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return nil, err
	}

	filename := strings.TrimSpace(filenameHint)
	if filename == "" {
		filename = name
	}

	return &ResolvedMedia{
		LocalPath: targetPath,
		Filename:  filename,
		MimeType:  resolvedMime,
	}, nil
}

func guessExtension(filenameHint, mimeType, sourceURL string) string {
	if ext := strings.TrimSpace(filepath.Ext(filenameHint)); ext != "" {
		return ext
	}
	if mimeType != "" {
		if exts, err := mime.ExtensionsByType(strings.Split(mimeType, ";")[0]); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	if parsed, err := url.Parse(sourceURL); err == nil {
		if ext := filepath.Ext(parsed.Path); ext != "" {
			return ext
		}
	}
	return ".bin"
}
