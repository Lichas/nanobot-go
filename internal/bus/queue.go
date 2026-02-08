package bus

import (
	"context"
	"sync"
)

// MessageBus 消息总线
type MessageBus struct {
	inbound  chan *InboundMessage
	outbound chan *OutboundMessage
	mu       sync.RWMutex
	closed   bool
}

// NewMessageBus 创建消息总线
func NewMessageBus(bufferSize int) *MessageBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &MessageBus{
		inbound:  make(chan *InboundMessage, bufferSize),
		outbound: make(chan *OutboundMessage, bufferSize),
	}
}

// PublishInbound 发布入站消息
func (b *MessageBus) PublishInbound(msg *InboundMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	select {
	case b.inbound <- msg:
		return nil
	default:
		return ErrBufferFull
	}
}

// PublishOutbound 发布出站消息
func (b *MessageBus) PublishOutbound(msg *OutboundMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	select {
	case b.outbound <- msg:
		return nil
	default:
		return ErrBufferFull
	}
}

// ConsumeInbound 消费入站消息（阻塞）
func (b *MessageBus) ConsumeInbound(ctx context.Context) (*InboundMessage, error) {
	select {
	case msg := <-b.inbound:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ConsumeOutbound 消费出站消息（阻塞）
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (*OutboundMessage, error) {
	select {
	case msg := <-b.outbound:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TryConsumeInbound 非阻塞消费入站消息
func (b *MessageBus) TryConsumeInbound() (*InboundMessage, bool) {
	select {
	case msg := <-b.inbound:
		return msg, true
	default:
		return nil, false
	}
}

// TryConsumeOutbound 非阻塞消费出站消息
func (b *MessageBus) TryConsumeOutbound() (*OutboundMessage, bool) {
	select {
	case msg := <-b.outbound:
		return msg, true
	default:
		return nil, false
	}
}

// Close 关闭消息总线
func (b *MessageBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.closed {
		b.closed = true
		close(b.inbound)
		close(b.outbound)
	}
}

// IsClosed 检查是否已关闭
func (b *MessageBus) IsClosed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}
