package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Task represents a single task in the task list
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // pending, in_progress, completed
	Priority    string    `json:"priority"` // low, medium, high
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Completed   time.Time `json:"completed,omitempty"`
	Subtasks    []Task    `json:"subtasks,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"` // IDs of tasks this depends on
}

// TaskList represents the entire task list
type TaskList struct {
	Tasks   []Task    `json:"tasks"`
	Updated time.Time `json:"updated"`
}

// getTaskListPath returns the path to the task list file
func getTaskListPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".config", "grokster")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "tasks.json"), nil
}

// loadTaskList loads the task list from disk
func loadTaskList() (*TaskList, error) {
	path, err := getTaskListPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TaskList{Tasks: []Task{}, Updated: time.Now()}, nil
		}
		return nil, err
	}

	var taskList TaskList
	if err := json.Unmarshal(data, &taskList); err != nil {
		return nil, err
	}

	return &taskList, nil
}

// saveTaskList saves the task list to disk
func saveTaskList(taskList *TaskList) error {
	path, err := getTaskListPath()
	if err != nil {
		return err
	}

	taskList.Updated = time.Now()
	data, err := json.MarshalIndent(taskList, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// generateTaskID generates a simple incremental task ID
func generateTaskID(tasks []Task) string {
	maxID := 0
	for _, task := range tasks {
		var id int
		fmt.Sscanf(task.ID, "task-%d", &id)
		if id > maxID {
			maxID = id
		}
	}
	return fmt.Sprintf("task-%d", maxID+1)
}

// TodoWriteTool creates or updates tasks
type TodoWriteTool struct {
	BaseTool
}

func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{
		BaseTool: BaseTool{
			name:              "todo_write",
			description:       "Create or update tasks in the task list",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *TodoWriteTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: create, update, delete",
				"enum":        []string{"create", "update", "delete"},
			},
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "Task ID (required for update/delete)",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Task title (required for create)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Task description",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Task status: pending, in_progress, completed",
				"enum":        []string{"pending", "in_progress", "completed"},
			},
			"priority": map[string]interface{}{
				"type":        "string",
				"description": "Task priority: low, medium, high",
				"enum":        []string{"low", "medium", "high"},
			},
			"subtasks": map[string]interface{}{
				"type":        "array",
				"description": "Array of subtask objects",
			},
			"dependencies": map[string]interface{}{
				"type":        "array",
				"description": "Array of task IDs this task depends on",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TodoWriteTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	taskList, err := loadTaskList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to load tasks: %v", err)}, err
	}

	switch action {
	case "create":
		title, ok := params["title"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "title is required for create"}, fmt.Errorf("title required")
		}

		task := Task{
			ID:          generateTaskID(taskList.Tasks),
			Title:       title,
			Description: getStringParam(params, "description", ""),
			Status:      getStringParam(params, "status", "pending"),
			Priority:    getStringParam(params, "priority", "medium"),
			Created:     time.Now(),
			Updated:     time.Now(),
		}

		// Handle dependencies
		if deps, ok := params["dependencies"].([]interface{}); ok {
			for _, dep := range deps {
				if depStr, ok := dep.(string); ok {
					task.Dependencies = append(task.Dependencies, depStr)
				}
			}
		}

		taskList.Tasks = append(taskList.Tasks, task)

		if err := saveTaskList(taskList); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save: %v", err)}, err
		}

		return &ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Created task %s: %s", task.ID, task.Title),
			Data:    map[string]interface{}{"task_id": task.ID},
		}, nil

	case "update":
		taskID, ok := params["task_id"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "task_id is required for update"}, fmt.Errorf("task_id required")
		}

		found := false
		for i := range taskList.Tasks {
			if taskList.Tasks[i].ID == taskID {
				found = true

				// Update fields if provided
				if title, ok := params["title"].(string); ok {
					taskList.Tasks[i].Title = title
				}
				if desc, ok := params["description"].(string); ok {
					taskList.Tasks[i].Description = desc
				}
				if status, ok := params["status"].(string); ok {
					taskList.Tasks[i].Status = status
					if status == "completed" {
						taskList.Tasks[i].Completed = time.Now()
					}
				}
				if priority, ok := params["priority"].(string); ok {
					taskList.Tasks[i].Priority = priority
				}

				taskList.Tasks[i].Updated = time.Now()
				break
			}
		}

		if !found {
			return &ToolResult{Success: false, Error: "task not found"}, fmt.Errorf("task not found")
		}

		if err := saveTaskList(taskList); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save: %v", err)}, err
		}

		return &ToolResult{Success: true, Output: fmt.Sprintf("Updated task %s", taskID)}, nil

	case "delete":
		taskID, ok := params["task_id"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "task_id is required for delete"}, fmt.Errorf("task_id required")
		}

		newTasks := []Task{}
		found := false
		for _, task := range taskList.Tasks {
			if task.ID != taskID {
				newTasks = append(newTasks, task)
			} else {
				found = true
			}
		}

		if !found {
			return &ToolResult{Success: false, Error: "task not found"}, fmt.Errorf("task not found")
		}

		taskList.Tasks = newTasks

		if err := saveTaskList(taskList); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save: %v", err)}, err
		}

		return &ToolResult{Success: true, Output: fmt.Sprintf("Deleted task %s", taskID)}, nil

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action")
	}
}

// TodoReadTool reads tasks from the task list
type TodoReadTool struct {
	BaseTool
}

func NewTodoReadTool() *TodoReadTool {
	return &TodoReadTool{
		BaseTool: BaseTool{
			name:              "todo_read",
			description:       "Read tasks from the task list with optional filtering",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *TodoReadTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "Specific task ID to retrieve (optional)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Filter by status: pending, in_progress, completed",
			},
			"priority": map[string]interface{}{
				"type":        "string",
				"description": "Filter by priority: low, medium, high",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Output format: json, table, summary (default: summary)",
				"enum":        []string{"json", "table", "summary"},
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TodoReadTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	taskList, err := loadTaskList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to load tasks: %v", err)}, err
	}

	// Filter tasks
	var filteredTasks []Task

	if taskID, ok := params["task_id"].(string); ok {
		for _, task := range taskList.Tasks {
			if task.ID == taskID {
				filteredTasks = []Task{task}
				break
			}
		}
	} else {
		filteredTasks = taskList.Tasks

		// Apply status filter
		if status, ok := params["status"].(string); ok {
			var temp []Task
			for _, task := range filteredTasks {
				if task.Status == status {
					temp = append(temp, task)
				}
			}
			filteredTasks = temp
		}

		// Apply priority filter
		if priority, ok := params["priority"].(string); ok {
			var temp []Task
			for _, task := range filteredTasks {
				if task.Priority == priority {
					temp = append(temp, task)
				}
			}
			filteredTasks = temp
		}
	}

	format := getStringParam(params, "format", "summary")

	var output string
	switch format {
	case "json":
		data, _ := json.MarshalIndent(filteredTasks, "", "  ")
		output = string(data)

	case "table":
		output = "ID         | Title                           | Status      | Priority | Updated\n"
		output += "-----------|--------------------------------|-------------|----------|-------------------------\n"
		for _, task := range filteredTasks {
			output += fmt.Sprintf("%-10s | %-30s | %-11s | %-8s | %s\n",
				task.ID,
				truncate(task.Title, 30),
				task.Status,
				task.Priority,
				task.Updated.Format("2006-01-02 15:04"),
			)
		}

	case "summary":
		if len(filteredTasks) == 0 {
			output = "No tasks found"
		} else {
			output = fmt.Sprintf("Found %d task(s):\n\n", len(filteredTasks))
			for _, task := range filteredTasks {
				statusIcon := "○"
				if task.Status == "in_progress" {
					statusIcon = "◐"
				} else if task.Status == "completed" {
					statusIcon = "●"
				}

				output += fmt.Sprintf("%s [%s] %s - %s\n", statusIcon, task.ID, task.Title, task.Status)
				if task.Description != "" {
					output += fmt.Sprintf("   %s\n", task.Description)
				}
				if len(task.Dependencies) > 0 {
					output += fmt.Sprintf("   Dependencies: %v\n", task.Dependencies)
				}
				output += "\n"
			}
		}
	}

	return &ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]interface{}{"tasks": filteredTasks, "count": len(filteredTasks)},
	}, nil
}

// CompleteTool marks a project as complete with a summary
type CompleteTool struct {
	BaseTool
}

func NewCompleteTool() *CompleteTool {
	return &CompleteTool{
		BaseTool: BaseTool{
			name:              "complete",
			description:       "Mark a project as complete with a summary and archive all tasks",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *CompleteTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Project completion summary",
			},
			"archive": map[string]interface{}{
				"type":        "boolean",
				"description": "Archive completed tasks (default: true)",
			},
		},
		"required": []string{"summary"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CompleteTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	summary, ok := params["summary"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "summary is required"}, fmt.Errorf("summary required")
	}

	archive := true
	if a, ok := params["archive"].(bool); ok {
		archive = a
	}

	taskList, err := loadTaskList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to load tasks: %v", err)}, err
	}

	// Count tasks by status
	stats := map[string]int{
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
	}

	for _, task := range taskList.Tasks {
		stats[task.Status]++
	}

	output := fmt.Sprintf("Project Completion Summary\n")
	output += fmt.Sprintf("==========================\n\n")
	output += fmt.Sprintf("%s\n\n", summary)
	output += fmt.Sprintf("Task Statistics:\n")
	output += fmt.Sprintf("  Completed: %d\n", stats["completed"])
	output += fmt.Sprintf("  In Progress: %d\n", stats["in_progress"])
	output += fmt.Sprintf("  Pending: %d\n", stats["pending"])
	output += fmt.Sprintf("  Total: %d\n\n", len(taskList.Tasks))

	if archive {
		// Archive to a completion file
		path, err := getTaskListPath()
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to get path: %v", err)}, err
		}

		archivePath := filepath.Join(filepath.Dir(path), fmt.Sprintf("tasks_archive_%s.json", time.Now().Format("2006-01-02_15-04-05")))

		data, _ := json.MarshalIndent(map[string]interface{}{
			"summary":   summary,
			"completed": time.Now(),
			"tasks":     taskList.Tasks,
			"stats":     stats,
		}, "", "  ")

		if err := os.WriteFile(archivePath, data, 0600); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to archive: %v", err)}, err
		}

		output += fmt.Sprintf("Tasks archived to: %s\n", archivePath)

		// Clear task list
		taskList.Tasks = []Task{}
		if err := saveTaskList(taskList); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save: %v", err)}, err
		}

		output += "Task list cleared\n"
	}

	return &ToolResult{Success: true, Output: output}, nil
}

// Helper functions

func getStringParam(params map[string]interface{}, key string, defaultVal string) string {
	if val, ok := params[key].(string); ok {
		return val
	}
	return defaultVal
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
