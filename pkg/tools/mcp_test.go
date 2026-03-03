package tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMCPClient struct {
	initErr  error
	listErr  error
	callErr  error
	closeErr error

	tools       []mcpRemoteTool
	callResults map[string]mcpCallResult

	initializeFn func(context.Context) error
	listToolsFn  func(context.Context) ([]mcpRemoteTool, error)
	callToolFn   func(context.Context, string, map[string]interface{}) (mcpCallResult, error)

	initializeCalled bool
	closeCalled      bool
	lastCallName     string
	lastCallArgs     map[string]interface{}
}

func (c *fakeMCPClient) Initialize(ctx context.Context) error {
	c.initializeCalled = true
	if c.initializeFn != nil {
		return c.initializeFn(ctx)
	}
	return c.initErr
}

func (c *fakeMCPClient) ListTools(ctx context.Context) ([]mcpRemoteTool, error) {
	if c.listToolsFn != nil {
		return c.listToolsFn(ctx)
	}
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.tools, nil
}

func (c *fakeMCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (mcpCallResult, error) {
	c.lastCallName = name
	c.lastCallArgs = args
	if c.callToolFn != nil {
		return c.callToolFn(ctx, name, args)
	}
	if c.callErr != nil {
		return mcpCallResult{}, c.callErr
	}
	if res, ok := c.callResults[name]; ok {
		return res, nil
	}
	return mcpCallResult{}, nil
}

func (c *fakeMCPClient) Close() error {
	c.closeCalled = true
	return c.closeErr
}

type fakeMCPFactory struct {
	clients map[string]*fakeMCPClient
	errs    map[string]error
}

func (f fakeMCPFactory) New(opts MCPServerOptions) (mcpClient, error) {
	if err, ok := f.errs[opts.Name]; ok {
		return nil, err
	}
	client, ok := f.clients[opts.Name]
	if !ok {
		return nil, assert.AnError
	}
	return client, nil
}

func TestMCPConnectorRegistersRemoteTools(t *testing.T) {
	reg := NewRegistry()
	client := &fakeMCPClient{
		tools: []mcpRemoteTool{
			{
				Name:        "search-web",
				Description: "Search from MCP",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"query"},
				},
			},
		},
		callResults: map[string]mcpCallResult{
			"search-web": {
				Content: []interface{}{
					map[string]interface{}{"type": "text", "text": "result line 1"},
					map[string]interface{}{"type": "text", "text": "result line 2"},
				},
			},
		},
	}

	connector := newMCPConnectorWithFactory(
		map[string]MCPServerOptions{
			"docs": {Command: "npx"},
		},
		fakeMCPFactory{
			clients: map[string]*fakeMCPClient{"docs": client},
			errs:    map[string]error{},
		},
	)

	err := connector.Connect(context.Background(), reg)
	require.NoError(t, err)
	assert.True(t, client.initializeCalled)
	assert.Equal(t, []string{"mcp_docs_search_web"}, connector.RegisteredTools())

	result, err := reg.Execute(context.Background(), "mcp_docs_search_web", map[string]interface{}{
		"query": "maxclaw",
	})
	require.NoError(t, err)
	assert.Equal(t, "result line 1\nresult line 2", result)
	assert.Equal(t, "search-web", client.lastCallName)
	assert.Equal(t, "maxclaw", client.lastCallArgs["query"])
}

func TestMCPConnectorContinuesWhenServerFails(t *testing.T) {
	reg := NewRegistry()
	working := &fakeMCPClient{
		tools: []mcpRemoteTool{
			{Name: "ping", Description: "pong", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		},
	}

	connector := newMCPConnectorWithFactory(
		map[string]MCPServerOptions{
			"broken":  {Command: "bad"},
			"working": {Command: "ok"},
		},
		fakeMCPFactory{
			clients: map[string]*fakeMCPClient{"working": working},
			errs:    map[string]error{"broken": assert.AnError},
		},
	)

	err := connector.Connect(context.Background(), reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
	assert.True(t, working.initializeCalled)

	_, ok := reg.Get("mcp_working_ping")
	assert.True(t, ok, "working server tools should still be registered")
}

func TestMCPConnectorCloseClosesClients(t *testing.T) {
	reg := NewRegistry()
	client := &fakeMCPClient{
		tools: []mcpRemoteTool{
			{Name: "ping", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		},
	}
	connector := newMCPConnectorWithFactory(
		map[string]MCPServerOptions{"x": {Command: "ok"}},
		fakeMCPFactory{
			clients: map[string]*fakeMCPClient{"x": client},
			errs:    map[string]error{},
		},
	)

	require.NoError(t, connector.Connect(context.Background(), reg))
	require.NoError(t, connector.Close())
	assert.True(t, client.closeCalled)
}

func TestRenderMCPToolResultStructuredContent(t *testing.T) {
	out := renderMCPToolResult(mcpCallResult{
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "ok"},
		},
		StructuredContent: map[string]interface{}{"k": "v"},
	})
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, `"k": "v"`)
}

func TestMCPConnectorConnectUsesDefaultTimeout(t *testing.T) {
	prevTimeout := defaultMCPConnectTimeout
	defaultMCPConnectTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		defaultMCPConnectTimeout = prevTimeout
	})

	reg := NewRegistry()
	blocking := &fakeMCPClient{
		initializeFn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	working := &fakeMCPClient{
		tools: []mcpRemoteTool{
			{Name: "ping", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		},
	}

	connector := newMCPConnectorWithFactory(
		map[string]MCPServerOptions{
			"blocking": {Command: "bad"},
			"working":  {Command: "ok"},
		},
		fakeMCPFactory{
			clients: map[string]*fakeMCPClient{
				"blocking": blocking,
				"working":  working,
			},
			errs: map[string]error{},
		},
	)

	start := time.Now()
	err := connector.Connect(context.Background(), reg)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocking: initialize failed")
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	assert.Less(t, elapsed, time.Second)
	assert.True(t, working.initializeCalled, "healthy server should still connect when another server times out")

	_, ok := reg.Get("mcp_working_ping")
	assert.True(t, ok)
}

func TestMCPToolWrapperExecuteUsesDefaultTimeout(t *testing.T) {
	prevTimeout := defaultMCPToolCallTimeout
	defaultMCPToolCallTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		defaultMCPToolCallTimeout = prevTimeout
	})

	client := &fakeMCPClient{
		callToolFn: func(ctx context.Context, _ string, _ map[string]interface{}) (mcpCallResult, error) {
			<-ctx.Done()
			return mcpCallResult{}, ctx.Err()
		},
	}

	tool := newMCPToolWrapper("slow", mcpRemoteTool{
		Name:        "hang",
		Description: "slow tool",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}, client)

	start := time.Now()
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	assert.Less(t, elapsed, time.Second)
}

func TestWithDefaultTimeout(t *testing.T) {
	t.Run("no deadline sets timeout", func(t *testing.T) {
		ctx, cancel := withDefaultTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(50*time.Millisecond), deadline, 20*time.Millisecond)
	})

	t.Run("long existing deadline uses shorter timeout", func(t *testing.T) {
		// 模拟 cron 任务的 10 分钟 deadline
		parentCtx, parentCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer parentCancel()

		// MCP 连接应该使用 8 秒超时（而不是 10 分钟）
		ctx, cancel := withDefaultTimeout(parentCtx, 8*time.Second)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		// 应该使用 8 秒超时
		remaining := time.Until(deadline)
		assert.Less(t, remaining, 10*time.Second)
		assert.Greater(t, remaining, 6*time.Second)
	})

	t.Run("short existing deadline keeps existing", func(t *testing.T) {
		// 设置一个短 deadline
		parentCtx, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer parentCancel()

		ctx, cancel := withDefaultTimeout(parentCtx, 5*time.Second)
		defer cancel()

		// 应该返回同一个 context（没有新创建）
		// 通过检查是否还是 parentCtx 来判断
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		remaining := time.Until(deadline)
		// 应该保持短的 deadline
		assert.Less(t, remaining, 200*time.Millisecond)
	})

	t.Run("zero timeout returns original context", func(t *testing.T) {
		ctx, cancel := withDefaultTimeout(context.Background(), 0)
		defer cancel()

		_, ok := ctx.Deadline()
		assert.False(t, ok)
	})
}

func TestWithDefaultTimeoutRespectsCronLongDeadline(t *testing.T) {
	// 这个测试模拟 Bug #2 的场景：
	// cron 任务给了 10 分钟 ctx，MCP 连接应该有 8 秒超时
	longTimeout := 10 * time.Minute
	mcpTimeout := 50 * time.Millisecond // 用短时间来加速测试

	parentCtx, parentCancel := context.WithTimeout(context.Background(), longTimeout)
	defer parentCancel()

	ctx, cancel := withDefaultTimeout(parentCtx, mcpTimeout)
	defer cancel()

	// 等待 MCP 超时
	select {
	case <-ctx.Done():
		// 正确：应该按照 MCP 的短超时触发
		assert.Equal(t, context.DeadlineExceeded, ctx.Err())
	case <-time.After(time.Second):
		t.Fatal("expected context to be canceled by MCP timeout, but it wasn't")
	}
}
