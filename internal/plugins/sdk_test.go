package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/velariumai/gorkbot/pkg/tools"
)

// MockTool is a test tool implementing tools.Tool interface
type MockTool struct {
	name        string
	category    tools.ToolCategory
	description string
}

func (mt *MockTool) Name() string                                                    { return mt.name }
func (mt *MockTool) Description() string                                             { return mt.description }
func (mt *MockTool) Category() tools.ToolCategory                                    { return mt.category }
func (mt *MockTool) Parameters() json.RawMessage                                     { return json.RawMessage(`{}`) }
func (mt *MockTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	return &tools.ToolResult{Success: true, Output: "mock result"}, nil
}
func (mt *MockTool) RequiresPermission() bool                                        { return false }
func (mt *MockTool) DefaultPermission() tools.PermissionLevel                        { return tools.PermissionOnce }
func (mt *MockTool) OutputFormat() tools.OutputFormat                                { return tools.FormatText }

// MockPlugin is a test plugin implementing GorkPlugin interface
type MockPlugin struct {
	name        string
	version     string
	tools       []tools.Tool
	initErr     error
	shutdownErr error
}

func (mp *MockPlugin) Name() string    { return mp.name }
func (mp *MockPlugin) Version() string { return mp.version }
func (mp *MockPlugin) Init(reg ToolRegistrar) error {
	if mp.initErr != nil {
		return mp.initErr
	}
	for _, tool := range mp.tools {
		if regErr := reg.Register(tool); regErr != nil {
			return regErr
		}
	}
	return nil
}
func (mp *MockPlugin) Shutdown() error { return mp.shutdownErr }

// TestToolRegistrar_Register tests ToolRegistrar implementation
func TestToolRegistrar_Register(t *testing.T) {
	permMgr, permErr := tools.NewPermissionManager(t.TempDir())
	if permErr != nil {
		t.Fatalf("NewPermissionManager failed: %v", permErr)
	}
	reg := tools.NewRegistry(permMgr)
	registrar := NewToolRegistrar(reg)

	tool := &MockTool{
		name:     "test_tool",
		category: tools.CategoryShell,
	}

	regErr := registrar.Register(tool)
	if regErr != nil {
		t.Fatalf("Register failed: %v", regErr)
	}

	// Verify tool was registered
	got, exists := reg.Get("test_tool")
	if !exists {
		t.Fatal("tool not found in registry")
	}
	if got.Name() != "test_tool" {
		t.Errorf("tool name mismatch: got %s, want test_tool", got.Name())
	}
}

// TestToolRegistrar_RegisterNilRegistry tests error handling for nil registry
func TestToolRegistrar_RegisterNilRegistry(t *testing.T) {
	registrar := &toolRegistrar{registry: nil}
	tool := &MockTool{name: "test"}

	regErr := registrar.Register(tool)
	if regErr != ErrNilRegistry {
		t.Errorf("expected ErrNilRegistry, got %v", regErr)
	}
}

// TestGorkPlugin_Init tests plugin initialization
func TestGorkPlugin_Init(t *testing.T) {
	permMgr, permErr := tools.NewPermissionManager(t.TempDir())
	if permErr != nil {
		t.Fatalf("NewPermissionManager failed: %v", permErr)
	}
	reg := tools.NewRegistry(permMgr)
	registrar := NewToolRegistrar(reg)

	plugin := &MockPlugin{
		name:    "test_plugin",
		version: "1.0.0",
		tools: []tools.Tool{
			&MockTool{name: "plugin_tool_1", category: tools.CategoryShell},
			&MockTool{name: "plugin_tool_2", category: tools.CategoryShell},
		},
	}

	initErr := plugin.Init(registrar)
	if initErr != nil {
		t.Fatalf("Init failed: %v", initErr)
	}

	// Verify both tools were registered
	for i := 1; i <= 2; i++ {
		toolNum := string(rune('0' + i))
		name := "plugin_tool_" + toolNum
		if _, exists := reg.Get(name); !exists {
			t.Errorf("tool %s not registered", name)
		}
	}
}

// TestGorkPlugin_InitError tests error handling during plugin initialization
func TestGorkPlugin_InitError(t *testing.T) {
	registrar := NewToolRegistrar(nil)
	plugin := &MockPlugin{
		name:    "error_plugin",
		version: "1.0.0",
		initErr: errors.New("custom init error"),
	}

	initErr := plugin.Init(registrar)
	if initErr == nil || initErr.Error() != "custom init error" {
		t.Errorf("expected custom init error, got %v", initErr)
	}
}

// TestGorkPlugin_RegisterPlugin tests Registry.RegisterPlugin integration
func TestGorkPlugin_RegisterPlugin(t *testing.T) {
	permMgr, permErr := tools.NewPermissionManager(t.TempDir())
	if permErr != nil {
		t.Fatalf("NewPermissionManager failed: %v", permErr)
	}
	reg := tools.NewRegistry(permMgr)

	tool := &MockTool{
		name:     "plugin_tool",
		category: tools.CategoryShell,
	}

	regErr := reg.RegisterPlugin(tool, nil)
	if regErr != nil {
		t.Fatalf("RegisterPlugin failed: %v", regErr)
	}

	// Verify tool was registered
	got, exists := reg.Get("plugin_tool")
	if !exists {
		t.Fatal("tool not found after RegisterPlugin")
	}
	if got.Name() != "plugin_tool" {
		t.Errorf("tool name mismatch: got %s, want plugin_tool", got.Name())
	}
}

// TestGorkPlugin_RegisterPluginInvalidType tests error handling for invalid plugin type
func TestGorkPlugin_RegisterPluginInvalidType(t *testing.T) {
	permMgr, permErr := tools.NewPermissionManager(t.TempDir())
	if permErr != nil {
		t.Fatalf("NewPermissionManager failed: %v", permErr)
	}
	reg := tools.NewRegistry(permMgr)

	regErr := reg.RegisterPlugin("not_a_tool", nil)
	if regErr == nil || !contains(regErr.Error(), "does not implement") {
		t.Errorf("expected type error, got %v", regErr)
	}
}

// Helper function to check if error message contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
