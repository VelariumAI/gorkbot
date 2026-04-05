package plugins

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"sync"

	"github.com/velariumai/gorkbot/pkg/tools"
)

// Common plugin errors
var (
	ErrNilRegistry = errors.New("registry is nil")
	ErrPluginNotFound = errors.New("plugin not found")
	ErrInvalidSymbol = errors.New("invalid plugin symbol")
	ErrInitFailed = errors.New("plugin initialization failed")
)

// ModuleMetadata describes a loaded module
type ModuleMetadata struct {
	Name        string
	Path        string
	Version     string
	Loaded      bool
	Error       error
	LoadedAt    int64
	Capabilities []string
}

// ModuleLoader dynamically loads .so plugin files
type ModuleLoader struct {
	logger        *slog.Logger
	pluginDir     string
	modules       map[string]*LoadedModule
	toolRegistry  *tools.Registry // Injected for plugin registration
	mu            sync.RWMutex
}

// LoadedModule represents an actively loaded module
type LoadedModule struct {
	Plugin      *plugin.Plugin
	Metadata    *ModuleMetadata
	Symbols     map[string]interface{}
}

// NewModuleLoader creates a new module loader
func NewModuleLoader(logger *slog.Logger, pluginDir string) *ModuleLoader {
	if logger == nil {
		logger = slog.Default()
	}

	if pluginDir == "" {
		home, _ := os.UserHomeDir()
		pluginDir = filepath.Join(home, ".config/gorkbot/plugins")
	}

	return &ModuleLoader{
		logger:    logger,
		pluginDir: pluginDir,
		modules:   make(map[string]*LoadedModule),
	}
}

// SetToolRegistry injects the tool registry for plugin registration.
func (ml *ModuleLoader) SetToolRegistry(reg *tools.Registry) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.toolRegistry = reg
}

// DiscoverModules discovers all .so files in plugin directory
func (ml *ModuleLoader) DiscoverModules() ([]string, error) {
	if _, err := os.Stat(ml.pluginDir); os.IsNotExist(err) {
		os.MkdirAll(ml.pluginDir, 0755)
		ml.logger.Info("created plugin directory", slog.String("path", ml.pluginDir))
		return nil, nil
	}

	entries, err := os.ReadDir(ml.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	var modules []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".so" {
			modules = append(modules, filepath.Join(ml.pluginDir, entry.Name()))
		}
	}

	ml.logger.Debug("discovered modules",
		slog.Int("count", len(modules)),
		slog.String("directory", ml.pluginDir),
	)

	return modules, nil
}

// LoadModule dynamically loads a .so plugin file
func (ml *ModuleLoader) LoadModule(modulePath string) (*LoadedModule, error) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	// Check if already loaded
	moduleName := filepath.Base(modulePath)
	if existing, ok := ml.modules[moduleName]; ok {
		return existing, nil
	}

	// Load the plugin
	p, err := plugin.Open(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin %s: %w", modulePath, err)
	}

	// Extract metadata symbol
	metadata := &ModuleMetadata{
		Name:     moduleName,
		Path:     modulePath,
		Loaded:   true,
		LoadedAt: int64(len(ml.modules)),
	}

	// Try to get module info
	if infoSym, err := p.Lookup("ModuleInfo"); err == nil {
		if info, ok := infoSym.(*ModuleMetadata); ok {
			metadata = info
		}
	}

	loaded := &LoadedModule{
		Plugin:   p,
		Metadata: metadata,
		Symbols:  make(map[string]interface{}),
	}

	// Load common symbols
	symbols := []string{"Init", "Shutdown", "Name", "Version"}
	for _, sym := range symbols {
		if symPtr, err := p.Lookup(sym); err == nil {
			loaded.Symbols[sym] = symPtr
		}
	}

	ml.modules[moduleName] = loaded

	// Attempt GorkPlugin initialization (Task 5.4)
	if gorkPluginSym, err := p.Lookup("GorkPlugin"); err == nil {
		if gorkPlugin, ok := gorkPluginSym.(GorkPlugin); ok {
			registrar := NewToolRegistrar(ml.toolRegistry)
			if initErr := gorkPlugin.Init(registrar); initErr != nil {
				ml.logger.Error("plugin initialization failed",
					slog.String("name", moduleName),
					slog.String("error", initErr.Error()),
				)
				metadata.Error = initErr
				metadata.Loaded = false
			} else {
				ml.logger.Info("plugin initialized successfully",
					slog.String("name", moduleName),
					slog.String("version", gorkPlugin.Version()),
				)
				metadata.Version = gorkPlugin.Version()
			}
		}
	}

	ml.logger.Info("loaded module",
		slog.String("name", moduleName),
		slog.String("path", modulePath),
		slog.Int("symbols", len(loaded.Symbols)),
	)

	return loaded, nil
}

// LoadAllModules loads all discovered modules
func (ml *ModuleLoader) LoadAllModules() []error {
	modules, err := ml.DiscoverModules()
	if err != nil {
		return []error{err}
	}

	var errors []error
	for _, modulePath := range modules {
		if _, err := ml.LoadModule(modulePath); err != nil {
			errors = append(errors, fmt.Errorf("module %s: %w", modulePath, err))
			ml.logger.Error("failed to load module",
				slog.String("path", modulePath),
				slog.String("error", err.Error()),
			)
		}
	}

	return errors
}

// GetModule retrieves a loaded module by name
func (ml *ModuleLoader) GetModule(name string) *LoadedModule {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	return ml.modules[name]
}

// ListModules returns all loaded modules
func (ml *ModuleLoader) ListModules() []*LoadedModule {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	modules := make([]*LoadedModule, 0, len(ml.modules))
	for _, m := range ml.modules {
		modules = append(modules, m)
	}
	return modules
}

// UnloadModule unloads a module (cleanup)
func (ml *ModuleLoader) UnloadModule(name string) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	module, ok := ml.modules[name]
	if !ok {
		return fmt.Errorf("module not found: %s", name)
	}

	// Call shutdown if available
	if shutdownSym, ok := module.Symbols["Shutdown"]; ok {
		if shutdownFunc, ok := shutdownSym.(func() error); ok {
			if err := shutdownFunc(); err != nil {
				ml.logger.Error("module shutdown error",
					slog.String("module", name),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	delete(ml.modules, name)

	ml.logger.Info("unloaded module", slog.String("name", name))

	return nil
}

// CallModuleFunc calls a function in a loaded module
func (ml *ModuleLoader) CallModuleFunc(moduleName string, funcName string, args ...interface{}) (interface{}, error) {
	module := ml.GetModule(moduleName)
	if module == nil {
		return nil, fmt.Errorf("module not found: %s", moduleName)
	}

	sym, ok := module.Symbols[funcName]
	if !ok {
		return nil, fmt.Errorf("function not found: %s.%s", moduleName, funcName)
	}

	// For now, we can't reliably call functions with arbitrary signatures
	// In practice, modules would implement known interfaces
	return sym, nil
}

// LoadAll loads all .so files from the plugin directory (Task 5.4).
func (ml *ModuleLoader) LoadAll() []error {
	modules, err := ml.DiscoverModules()
	if err != nil {
		return []error{fmt.Errorf("discovery failed: %w", err)}
	}

	var errors []error
	for _, modulePath := range modules {
		if _, err := ml.LoadModule(modulePath); err != nil {
			errors = append(errors, fmt.Errorf("module %s: %w", modulePath, err))
		}
	}

	return errors
}

// GetStats returns loader statistics
func (ml *ModuleLoader) GetStats() map[string]interface{} {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	return map[string]interface{}{
		"loaded_modules": len(ml.modules),
		"plugin_dir":     ml.pluginDir,
	}
}
