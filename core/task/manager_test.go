package task

import (
	"errors"
	"testing"
)

// TestManager_StateMachine 模拟任务从创建到完成的状态流转。
func TestManager_StateMachine(t *testing.T) {
	m := NewManager()
	task := m.CreateTask(TaskTypeDownload)

	if task.GetStatus() != TaskStatusPending {
		t.Fatalf("初始状态应为 pending，实际 %s", task.GetStatus())
	}

	task.SetStatus(TaskStatusRunning)
	if task.GetStatus() != TaskStatusRunning {
		t.Fatalf("应进入 running，实际 %s", task.GetStatus())
	}

	if err := m.Pause(task.ID); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	if status := mustStatus(t, m, task.ID); status != TaskStatusPaused {
		t.Fatalf("暂停后状态应为 paused，实际 %s", status)
	}

	if err := m.Resume(task.ID); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if status := mustStatus(t, m, task.ID); status != TaskStatusPending {
		t.Fatalf("恢复后应回到 pending，实际 %s", status)
	}

	task.SetStatus(TaskStatusRunning)
	task.SetStatus(TaskStatusCompleted)
	if status := mustStatus(t, m, task.ID); status != TaskStatusCompleted {
		t.Fatalf("完成后状态应为 completed，实际 %s", status)
	}

	if err := m.RemoveTask(task.ID); err != nil {
		t.Fatalf("完成任务应允许移除: %v", err)
	}
}

// TestManager_PauseAndResume 验证暂停与恢复的合法性。
func TestManager_PauseAndResume(t *testing.T) {
	m := NewManager()
	task := m.CreateTask(TaskTypeUpload)
	task.SetStatus(TaskStatusRunning)

	if err := m.Pause(task.ID); err != nil {
		t.Fatalf("暂停失败: %v", err)
	}
	if status := mustStatus(t, m, task.ID); status != TaskStatusPaused {
		t.Fatalf("暂停后状态应为 paused，实际 %s", status)
	}

	if err := m.Resume(task.ID); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if status := mustStatus(t, m, task.ID); status != TaskStatusPending {
		t.Fatalf("恢复后应为 pending，实际 %s", status)
	}
}

// TestManager_CancelTask 验证取消后的状态与二次取消。
func TestManager_CancelTask(t *testing.T) {
	m := NewManager()
	task := m.CreateTask(TaskTypeDownload)
	task.SetStatus(TaskStatusRunning)

	if err := m.Cancel(task.ID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	if status := mustStatus(t, m, task.ID); status != TaskStatusCanceled {
		t.Fatalf("取消后状态应为 canceled，实际 %s", status)
	}

	if err := m.Cancel(task.ID); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("重复取消应返回无效状态错误，实际: %v", err)
	}

	if err := m.RemoveTask(task.ID); err != nil {
		t.Fatalf("取消任务应允许移除: %v", err)
	}
}

func mustStatus(t *testing.T, m *Manager, id string) TaskStatus {
	t.Helper()
	task, err := m.GetTask(id)
	if err != nil {
		t.Fatalf("获取任务失败: %v", err)
	}
	return task.GetStatus()
}
