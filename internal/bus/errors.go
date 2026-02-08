package bus

import "errors"

var (
	// ErrBusClosed 消息总线已关闭
	ErrBusClosed = errors.New("message bus is closed")
	// ErrBufferFull 缓冲区已满
	ErrBufferFull = errors.New("message buffer is full")
)
