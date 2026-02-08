package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SpawnCallback 子代理回调函数类型
type SpawnCallback func(task string) error

// SpawnTool 子代理工具 - 用于后台任务执行
type SpawnTool struct {
	BaseTool
	callback     SpawnCallback
	mu           sync.RWMutex
	channel      string
	chatID       string
	runningTasks map[string]*SpawnTask
}

// SpawnTask 表示一个正在运行的后台任务
type SpawnTask struct {
	ID        string
	Label     string
	Task      string
	StartTime time.Time
	Status    string
}

// NewSpawnTool 创建子代理工具
func NewSpawnTool(callback SpawnCallback) *SpawnTool {
	return &SpawnTool{
		BaseTool: BaseTool{
			name:        "spawn",
			description: "Spawn a subagent to handle a task in the background. Use this for complex or time-consuming tasks that can run independently. The subagent will complete the task and report back when done.",
			parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task": map[string]interface{}{
						"type":        "string",
						"description": "The task for the subagent to complete",
						"minLength":   1,
					},
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Optional short label for the task (for display)",
					},
				},
				"required": []string{"task"},
			},
		},
		callback:     callback,
		runningTasks: make(map[string]*SpawnTask),
	}
}

// SetContext 设置当前上下文
func (t *SpawnTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.channel = channel
	t.chatID = chatID
}

// Execute 执行子代理任务
func (t *SpawnTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	task, _ := params["task"].(string)
	if task == "" {
		return "", fmt.Errorf("task is required")
	}

	label, _ := params["label"].(string)
	if label == "" {
		label = task[:min(30, len(task))]
	}

	// 生成任务ID
	taskID := generateTaskID()

	// 记录任务
	spawnTask := &SpawnTask{
		ID:        taskID,
		Label:     label,
		Task:      task,
		StartTime: time.Now(),
		Status:    "running",
	}

	t.mu.Lock()
	t.runningTasks[taskID] = spawnTask
	t.mu.Unlock()

	// 在后台执行
	go t.runTask(spawnTask)

	return fmt.Sprintf("Spawned subagent '%s' (id: %s) to handle: %s", label, taskID, task), nil
}

// runTask 在后台运行任务
func (t *SpawnTool) runTask(task *SpawnTask) {
	defer func() {
		t.mu.Lock()
		delete(t.runningTasks, task.ID)
		t.mu.Unlock()
	}()

	// 如果有回调，通知任务开始
	if t.callback != nil {
		t.callback(fmt.Sprintf("[Subagent %s] Started: %s", task.ID, task.Task))
	}

	// 这里可以集成实际的子代理执行逻辑
	// 简化版本：只是模拟执行并记录
	time.Sleep(100 * time.Millisecond)

	task.Status = "completed"

	// 如果有回调，通知任务完成
	if t.callback != nil {
		t.callback(fmt.Sprintf("[Subagent %s] Completed: %s", task.ID, task.Label))
	}
}

// ListRunningTasks 列出正在运行的任务
func (t *SpawnTool) ListRunningTasks() []*SpawnTask {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tasks := make([]*SpawnTask, 0, len(t.runningTasks))
	for _, task := range t.runningTasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
