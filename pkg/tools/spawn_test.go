package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnTool(t *testing.T) {
	var receivedTask string
	callback := func(task string) error {
		receivedTask = task
		return nil
	}
	_ = receivedTask // 避免未使用错误

	tool := NewSpawnTool(callback)
	tool.SetContext("telegram", "123456")
	ctx := context.Background()

	t.Run("spawn simple task", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"task": "do something in background",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "Spawned subagent")
		assert.Contains(t, result, "do something in background")
	})

	t.Run("spawn with label", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"task":  "analyze large dataset",
			"label": "Data Analysis",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "Spawned subagent")
		assert.Contains(t, result, "Data Analysis")
	})

	t.Run("missing task", func(t *testing.T) {
		_, err := tool.Execute(ctx, map[string]interface{}{
			"label": "No task provided",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task is required")
	})
}

func TestSpawnToolListRunning(t *testing.T) {
	tool := NewSpawnTool(nil)

	// 初始时应该没有运行中的任务
	tasks := tool.ListRunningTasks()
	assert.Empty(t, tasks)

	// 启动一个任务
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]interface{}{
		"task": "long running task",
	})
	require.NoError(t, err)

	// 应该有一个运行中的任务（短暂时间内）
	time.Sleep(50 * time.Millisecond)
	tasks = tool.ListRunningTasks()
	assert.Len(t, tasks, 1)
	assert.Equal(t, "long running task", tasks[0].Task)
	assert.Equal(t, "running", tasks[0].Status)

	// 等待任务完成
	time.Sleep(200 * time.Millisecond)
	tasks = tool.ListRunningTasks()
	assert.Empty(t, tasks)
}

func TestSpawnToolLongTaskTruncation(t *testing.T) {
	tool := NewSpawnTool(nil)
	ctx := context.Background()

	// 创建一个很长的任务描述
	longTask := strings.Repeat("a", 100)
	result, err := tool.Execute(ctx, map[string]interface{}{
		"task": longTask,
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Spawned subagent")
}
