package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// JobFunc 任务执行函数类型
type JobFunc func(job *Job) (string, error)

// Service 定时任务服务
type Service struct {
	jobs      map[string]*Job
	mu        sync.RWMutex
	storePath string
	running   bool
	stopChan  chan struct{}
	wg        sync.WaitGroup
	onJob     JobFunc
	cron      *cron.Cron
}

// NewService 创建定时任务服务
func NewService(storePath string) *Service {
	s := &Service{
		jobs:      make(map[string]*Job),
		storePath: storePath,
		stopChan:  make(chan struct{}),
		cron:      cron.New(),
	}
	s.load()
	return s
}

// SetJobHandler 设置任务处理器
func (s *Service) SetJobHandler(handler JobFunc) {
	s.onJob = handler
}

// AddJob 添加任务
func (s *Service) AddJob(name string, schedule Schedule, payload Payload) (*Job, error) {
	job := NewJob(name, schedule, payload)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[job.ID] = job

	// 如果服务正在运行，立即调度
	if s.running {
		s.scheduleJob(job)
	}

	if err := s.save(); err != nil {
		delete(s.jobs, job.ID)
		return nil, fmt.Errorf("failed to save job: %w", err)
	}

	return job, nil
}

// GetJob 获取任务
func (s *Service) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

// RemoveJob 删除任务
func (s *Service) RemoveJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		return false
	}

	delete(s.jobs, id)
	s.save()
	return true
}

// ListJobs 列出所有任务
func (s *Service) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// EnableJob 启用/禁用任务
func (s *Service) EnableJob(id string, enabled bool) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}

	job.Enabled = enabled
	s.save()
	return job, true
}

// Start 启动服务
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("service already running")
	}

	s.running = true
	s.stopChan = make(chan struct{})

	// 调度所有已启用的任务
	for _, job := range s.jobs {
		if job.Enabled {
			s.scheduleJob(job)
		}
	}

	s.cron.Start()

	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopChan)
	s.cron.Stop()
	s.wg.Wait()
}

// IsRunning 检查是否在运行
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// scheduleJob 调度单个任务
func (s *Service) scheduleJob(job *Job) {
	switch job.Schedule.Type {
	case ScheduleTypeEvery:
		s.scheduleEveryJob(job)
	case ScheduleTypeCron:
		s.scheduleCronJob(job)
	case ScheduleTypeOnce:
		s.scheduleOnceJob(job)
	}
}

// scheduleEveryJob 调度周期性任务
func (s *Service) scheduleEveryJob(job *Job) {
	duration := time.Duration(job.Schedule.EveryMs) * time.Millisecond

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if !s.running {
					return
				}
				s.executeJob(job)
			case <-s.stopChan:
				return
			}
		}
	}()
}

// scheduleCronJob 调度 Cron 任务
func (s *Service) scheduleCronJob(job *Job) {
	if job.Schedule.Expr == "" {
		return
	}

	s.cron.AddFunc(job.Schedule.Expr, func() {
		s.executeJob(job)
	})
}

// scheduleOnceJob 调度一次性任务
func (s *Service) scheduleOnceJob(job *Job) {
	at := time.UnixMilli(job.Schedule.AtMs)
	if at.Before(time.Now()) {
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		select {
		case <-time.After(time.Until(at)):
			if s.running {
				s.executeJob(job)
			}
		case <-s.stopChan:
			return
		}
	}()
}

// executeJob 执行任务
func (s *Service) executeJob(job *Job) {
	if !job.Enabled || s.onJob == nil {
		return
	}

	fmt.Printf("[Cron] Executing job: %s (%s)\n", job.Name, job.ID)
	result, err := s.onJob(job)
	if err != nil {
		fmt.Printf("[Cron] Job failed: %s, error: %v\n", job.Name, err)
	} else {
		fmt.Printf("[Cron] Job completed: %s, result: %s\n", job.Name, result)
	}
}

// save 保存任务到文件
func (s *Service) save() error {
	if s.storePath == "" {
		return nil
	}

	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.jobs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.storePath, data, 0644)
}

// load 从文件加载任务
func (s *Service) load() error {
	if s.storePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var jobs map[string]*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return err
	}

	s.jobs = jobs
	return nil
}

// Status 获取服务状态
func (s *Service) Status() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	enabledCount := 0
	for _, job := range s.jobs {
		if job.Enabled {
			enabledCount++
		}
	}

	return map[string]interface{}{
		"running":     s.running,
		"totalJobs":   len(s.jobs),
		"enabledJobs": enabledCount,
		"storePath":   s.storePath,
	}
}
