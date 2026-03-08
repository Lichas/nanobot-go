package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lichas/maxclaw/internal/bus"
	"github.com/Lichas/maxclaw/internal/logging"
)

// TelegramConfig Telegram 配置
type TelegramConfig struct {
	Token     string   `json:"token"`
	Enabled   bool     `json:"enabled"`
	AllowFrom []string `json:"allowFrom,omitempty"`
	Proxy     string   `json:"proxy,omitempty"`
}

// TelegramChannel Telegram 频道
type TelegramChannel struct {
	config         *TelegramConfig
	httpClient     *http.Client
	messageHandler func(msg *Message)
	stopChan       chan struct{}
	wg             sync.WaitGroup
	offset         int64
	enabled        bool
	mu             sync.RWMutex
	status         string
	botUsername    string
	botName        string
	lastError      string
}

type telegramGetUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

type telegramUpdate struct {
	UpdateID int64           `json:"update_id"`
	Message  telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int64             `json:"message_id"`
	From      telegramUser      `json:"from"`
	Chat      telegramChat      `json:"chat"`
	Text      string            `json:"text"`
	Caption   string            `json:"caption"`
	Photo     []telegramPhoto   `json:"photo"`
	Document  *telegramDocument `json:"document"`
	Date      int64             `json:"date"`
}

type telegramUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramPhoto struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type telegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
}

// NewTelegramChannel 创建 Telegram 频道
func NewTelegramChannel(config *TelegramConfig) *TelegramChannel {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if proxy := strings.TrimSpace(config.Proxy); proxy != "" {
		if parsed, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}

	return &TelegramChannel{
		config: config,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		stopChan: make(chan struct{}),
		offset:   0,
		enabled:  config.Enabled && config.Token != "",
	}
}

// Name 返回频道名称
func (t *TelegramChannel) Name() string {
	return "telegram"
}

// IsEnabled 是否启用
func (t *TelegramChannel) IsEnabled() bool {
	return t.enabled
}

// SetMessageHandler 设置消息处理器
func (t *TelegramChannel) SetMessageHandler(handler func(msg *Message)) {
	t.messageHandler = handler
}

// Start 启动 Telegram 频道
func (t *TelegramChannel) Start(ctx context.Context) error {
	if !t.enabled {
		return nil
	}

	t.refreshBotInfo()

	t.wg.Add(1)
	go t.pollUpdates(ctx)

	return nil
}

// Stop 停止 Telegram 频道
func (t *TelegramChannel) Stop() error {
	if !t.enabled {
		return nil
	}

	close(t.stopChan)
	t.wg.Wait()
	return nil
}

// pollUpdates 轮询更新
func (t *TelegramChannel) pollUpdates(ctx context.Context) {
	defer t.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		case <-ticker.C:
			t.fetchUpdates()
		}
	}
}

// fetchUpdates 获取更新
func (t *TelegramChannel) fetchUpdates() {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", t.config.Token)

	params := url.Values{}
	params.Set("offset", strconv.FormatInt(t.offset+1, 10))
	params.Set("limit", "100")

	resp, err := t.httpClient.Get(apiURL + "?" + params.Encode())
	if err != nil {
		st := t.Status()
		t.setStatus("error", st.Username, st.Name, err.Error())
		if lg := logging.Get(); lg != nil && lg.Channels != nil {
			lg.Channels.Printf("telegram getUpdates error: %v", err)
		}
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		st := t.Status()
		t.setStatus("error", st.Username, st.Name, err.Error())
		if lg := logging.Get(); lg != nil && lg.Channels != nil {
			lg.Channels.Printf("telegram readUpdates error: %v", err)
		}
		return
	}

	var result telegramGetUpdatesResponse

	if err := json.Unmarshal(body, &result); err != nil {
		st := t.Status()
		t.setStatus("error", st.Username, st.Name, err.Error())
		if lg := logging.Get(); lg != nil && lg.Channels != nil {
			lg.Channels.Printf("telegram parseUpdates error: %v", err)
		}
		return
	}

	if !result.OK {
		st := t.Status()
		t.setStatus("error", st.Username, st.Name, "getUpdates returned not ok")
		if lg := logging.Get(); lg != nil && lg.Channels != nil {
			lg.Channels.Printf("telegram getUpdates not ok")
		}
		return
	}

	st := t.Status()
	if st.Status != "ready" {
		t.setStatus("ready", st.Username, st.Name, "")
	}

	for _, update := range result.Result {
		if update.UpdateID > t.offset {
			t.offset = update.UpdateID
		}

		if t.messageHandler != nil {
			msg := t.buildInboundMessage(update.Message)
			if msg == nil {
				continue
			}
			msg.Raw = update
			t.messageHandler(msg)
			if lg := logging.Get(); lg != nil && lg.Channels != nil {
				if msg.Media != nil && msg.Media.Type != "" {
					lg.Channels.Printf("telegram inbound chat=%s sender=%s text=%q media=%s", msg.ChatID, msg.Sender, logging.Truncate(msg.Text, 300), msg.Media.Type)
				} else {
					lg.Channels.Printf("telegram inbound chat=%s sender=%s text=%q", msg.ChatID, msg.Sender, logging.Truncate(msg.Text, 300))
				}
			}
		}
	}
}

func (t *TelegramChannel) buildInboundMessage(message telegramMessage) *Message {
	if !t.isAllowed(message.From.ID, message.From.Username) {
		return nil
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = strings.TrimSpace(message.Caption)
	}

	media := telegramInboundMedia(message)
	if text == "" && media != nil {
		switch media.Type {
		case "image":
			text = "[Image]"
		case "document":
			text = "[Document]"
		default:
			text = "[Attachment]"
		}
	}
	if text == "" {
		return nil
	}

	sender := message.From.Username
	if strings.TrimSpace(sender) == "" {
		sender = strconv.FormatInt(message.From.ID, 10)
	}

	return &Message{
		ID:      strconv.FormatInt(message.MessageID, 10),
		Text:    text,
		Sender:  sender,
		ChatID:  strconv.FormatInt(message.Chat.ID, 10),
		Channel: "telegram",
		Media:   media,
	}
}

func telegramInboundMedia(message telegramMessage) *bus.MediaAttachment {
	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		return &bus.MediaAttachment{
			Type:     "image",
			FileID:   strings.TrimSpace(photo.FileID),
			MimeType: "image/jpeg",
		}
	}

	if message.Document == nil {
		return nil
	}

	mimeType := strings.TrimSpace(message.Document.MimeType)
	mediaType := "document"
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		mediaType = "image"
	}

	return &bus.MediaAttachment{
		Type:     mediaType,
		FileID:   strings.TrimSpace(message.Document.FileID),
		Filename: strings.TrimSpace(message.Document.FileName),
		MimeType: mimeType,
	}
}

func (t *TelegramChannel) isAllowed(userID int64, username string) bool {
	if len(t.config.AllowFrom) == 0 {
		return true
	}

	idStr := strconv.FormatInt(userID, 10)
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")

	for _, allowed := range t.config.AllowFrom {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if allowed == idStr {
			return true
		}
		if strings.TrimPrefix(allowed, "@") == username && username != "" {
			return true
		}
	}
	return false
}

// SendMessage 发送消息
func (t *TelegramChannel) SendMessage(chatID string, text string) error {
	if !t.enabled {
		return fmt.Errorf("telegram channel not enabled")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.config.Token)

	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", html.EscapeString(text))
	params.Set("parse_mode", "HTML")

	resp, err := t.httpClient.Post(
		apiURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %s", string(body))
	}

	if lg := logging.Get(); lg != nil && lg.Channels != nil {
		lg.Channels.Printf("telegram send chat=%s text=%q", chatID, logging.Truncate(text, 300))
	}
	return nil
}

// SendPhoto 发送图片
func (t *TelegramChannel) SendPhoto(chatID string, photoPath string, caption string) error {
	if !t.enabled {
		return fmt.Errorf("telegram channel not enabled")
	}

	return t.sendFile(chatID, photoPath, "sendPhoto", "photo", caption)
}

// SendDocument 发送文档
func (t *TelegramChannel) SendDocument(chatID string, docPath string, caption string) error {
	if !t.enabled {
		return fmt.Errorf("telegram channel not enabled")
	}

	return t.sendFile(chatID, docPath, "sendDocument", "document", caption)
}

// sendFile 发送文件通用方法
func (t *TelegramChannel) sendFile(chatID, filePath, apiMethod, fileField, caption string) error {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	// 获取文件信息（用于验证文件存在）
	_, err = file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// 创建 multipart 请求体
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加 chat_id
	if err := writer.WriteField("chat_id", chatID); err != nil {
		return fmt.Errorf("failed to write chat_id field: %w", err)
	}

	// 添加 caption（如果有）
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return fmt.Errorf("failed to write caption field: %w", err)
		}
	}

	// 添加文件
	part, err := writer.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	// 复制文件内容
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// 关闭 writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// 发送请求
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.config.Token, apiMethod)
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// 解析响应验证
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram API returned not ok: %s", string(respBody))
	}

	if lg := logging.Get(); lg != nil && lg.Channels != nil {
		lg.Channels.Printf("telegram send file chat=%s file=%s type=%s", chatID, filepath.Base(filePath), fileField)
	}

	return nil
}

// TelegramStatus 当前状态快照
type TelegramStatus struct {
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status,omitempty"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
	Link     string `json:"link,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Status 返回 Telegram 频道状态（供 Web UI 使用）
func (t *TelegramChannel) Status() TelegramStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	link := ""
	if t.botUsername != "" {
		link = fmt.Sprintf("https://t.me/%s", t.botUsername)
	}

	return TelegramStatus{
		Enabled:  t.enabled,
		Status:   t.status,
		Username: t.botUsername,
		Name:     t.botName,
		Link:     link,
		Error:    t.lastError,
	}
}

func (t *TelegramChannel) refreshBotInfo() {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", t.config.Token)

	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		t.setStatus("error", "", "", err.Error())
		return
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.setStatus("error", "", "", err.Error())
		return
	}
	if !result.OK {
		if result.Description == "" {
			result.Description = "invalid token"
		}
		t.setStatus("error", "", "", result.Description)
		return
	}

	t.setStatus("ready", result.Result.Username, result.Result.FirstName, "")
}

func (t *TelegramChannel) setStatus(status, username, name, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status = status
	t.botUsername = username
	t.botName = name
	t.lastError = errMsg
}
