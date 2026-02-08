package channels

import (
	"context"
)

// Message 频道消息
type Message struct {
	ID      string
	Text    string
	Sender  string
	ChatID  string
	Channel string
	Raw     interface{}
}

// Channel 频道接口
type Channel interface {
	// Name 返回频道名称
	Name() string
	// Start 启动频道
	Start(ctx context.Context) error
	// Stop 停止频道
	Stop() error
	// SendMessage 发送消息
	SendMessage(chatID string, text string) error
	// SetMessageHandler 设置消息处理器
	SetMessageHandler(handler func(msg *Message))
	// IsEnabled 是否启用
	IsEnabled() bool
}

// Registry 频道注册表
type Registry struct {
	channels map[string]Channel
}

// NewRegistry 创建频道注册表
func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Channel),
	}
}

// Register 注册频道
func (r *Registry) Register(channel Channel) {
	r.channels[channel.Name()] = channel
}

// Get 获取频道
func (r *Registry) Get(name string) (Channel, bool) {
	ch, ok := r.channels[name]
	return ch, ok
}

// GetAll 获取所有频道
func (r *Registry) GetAll() []Channel {
	channels := make([]Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	return channels
}

// GetEnabled 获取启用的频道
func (r *Registry) GetEnabled() []Channel {
	channels := make([]Channel, 0)
	for _, ch := range r.channels {
		if ch.IsEnabled() {
			channels = append(channels, ch)
		}
	}
	return channels
}
