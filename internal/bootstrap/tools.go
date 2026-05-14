package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/process"
	"github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/research"
	"github.com/velariumai/gorkbot/pkg/researchgate"
	"github.com/velariumai/gorkbot/pkg/scheduler"
	"github.com/velariumai/gorkbot/pkg/selfmod"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/subagents"
	"github.com/velariumai/gorkbot/pkg/tools"
	"github.com/velariumai/gorkbot/pkg/usercommands"
)

type ToolSetupOptions struct {
	ConfigDir                   string
	Logger                      *slog.Logger
	KeyStore                    *providers.KeyStore
	Primary                     ai.AIProvider
	Consultant                  ai.AIProvider
	ResearchEgressMode          string
	ResearchMaxResponseBytes    int64
	ResearchTimeout             time.Duration
	ResearchAllowPrivateNetwork bool
	ResearchAllowCredentials    bool
}

type ToolSetup struct {
	ProcessManager      *process.Manager
	ToolRegistry        *tools.Registry
	SenseInputSanitizer *sense.InputSanitizer
	SenseTracer         *sense.SENSETracer
	ResearchEngine      *research.Engine
	ResearchGateway     *researchgate.Gateway
	SubAgentManager     *subagents.Manager
	DiscoveryManager    *discovery.Manager
	SchedulerStore      *scheduler.Store
	Scheduler           *scheduler.Scheduler
	UserCommandLoader   *usercommands.Loader
	Cleanup             func()
}

func SetupTools(opts ToolSetupOptions) (*ToolSetup, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	processManager := process.NewManager()

	permissionMgr, err := tools.NewPermissionManager(opts.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize permission manager", "error", err)
	}

	analytics, err := tools.NewAnalytics(opts.ConfigDir)
	if err != nil {
		logger.Error("Failed to initialize analytics", "error", err)
	}

	auditPruneCtx, auditPruneCancel := context.WithCancel(context.Background())
	auditDB, auditErr := tools.InitAuditDB(opts.ConfigDir)
	if auditErr != nil {
		logger.Warn("Audit DB init failed — tool executions will not be logged to audit.db",
			"error", auditErr)
	}
	if auditDB != nil {
		auditDB.StartPruner(auditPruneCtx, tools.DefaultAuditMaxRecords)
		logger.Info("Audit DB initialized", "path", filepath.Join(opts.ConfigDir, "audit.db"))
	}

	toolRegistry := tools.NewRegistry(permissionMgr)
	toolRegistry.SetAnalytics(analytics)
	toolRegistry.SetAIProvider(opts.Primary)
	toolRegistry.SetConsultantProvider(opts.Consultant)
	toolRegistry.SetConfigDir(opts.ConfigDir)
	if auditDB != nil {
		toolRegistry.SetAuditDB(auditDB)
	}

	senseInputSanitizer, sanitizerErr := sense.NewInputSanitizer()
	if sanitizerErr != nil {
		logger.Warn("SENSE input sanitizer init failed — proceeding without sanitization",
			"error", sanitizerErr)
	} else {
		toolRegistry.SetInputSanitizer(senseInputSanitizer)
		logger.Info("SENSE input sanitizer active", "cwd", senseInputSanitizer.CWD())
	}

	senseTraceDir := filepath.Join(opts.ConfigDir, "sense", "traces")
	senseSessionID := fmt.Sprintf("%d", time.Now().Unix())
	senseTracer := sense.NewSENSETracer(senseTraceDir, senseSessionID)
	toolRegistry.SetSENSETracer(senseTracer)
	toolRegistry.SetTraceSink(senseTracer.CanonicalSink(), senseTracer.CanonicalMode())
	selfmod.SetTraceSink(senseTracer.CanonicalSink(), senseTracer.CanonicalMode())
	logger.Info("SENSE tracer active", "trace_dir", senseTraceDir)

	researchEngine := research.NewEngine(10, logger)
	toolRegistry.SetResearchEngine(researchEngine)
	logger.Info("Research engine initialized", "max_documents", 10)

	researchPolicy := researchgate.DefaultPolicy()
	researchPolicy.MaxResponseBytes = opts.ResearchMaxResponseBytes
	researchPolicy.DefaultTimeout = opts.ResearchTimeout
	researchPolicy.AllowPrivateNetworks = opts.ResearchAllowPrivateNetwork
	researchPolicy.AllowCredentials = opts.ResearchAllowCredentials
	researchPolicy.MaxTimeout = 20 * time.Second
	researchGateway := researchgate.New(researchPolicy, logger)
	toolRegistry.SetResearchGateway(researchGateway, opts.ResearchEgressMode)
	researchGateway.SetTraceSink(senseTracer.CanonicalSink(), senseTracer.CanonicalMode())
	researchEngine.SetGateway(researchGateway, opts.ResearchEgressMode)
	logger.Info("Research egress configured",
		"mode", opts.ResearchEgressMode,
		"max_response_bytes", researchPolicy.MaxResponseBytes,
		"default_timeout", researchPolicy.DefaultTimeout.String(),
		"allow_private_network", researchPolicy.AllowPrivateNetworks,
		"allow_credentials", researchPolicy.AllowCredentials,
	)

	if err := toolRegistry.RegisterDefaultTools(); err != nil {
		if auditDB != nil {
			auditDB.Close()
		}
		senseTracer.Close()
		auditPruneCancel()
		return nil, err
	}

	postNotifyTool := tools.NewPostNotifyTool(tools.NewNotificationRouter(nil, nil, "", 0))
	if err := toolRegistry.Register(postNotifyTool); err != nil {
		logger.Debug("post_notify already registered", "error", err)
	}

	if err := toolRegistry.Register(tools.NewStartManagedProcessTool(processManager)); err != nil {
		logger.Warn("Failed to register StartManagedProcessTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewListManagedProcessesTool(processManager)); err != nil {
		logger.Warn("Failed to register ListManagedProcessesTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewStopManagedProcessTool(processManager)); err != nil {
		logger.Warn("Failed to register StopManagedProcessTool", "error", err)
	}
	if err := toolRegistry.Register(tools.NewReadManagedProcessOutputTool(processManager)); err != nil {
		logger.Warn("Failed to register ReadManagedProcessOutputTool", "error", err)
	}

	subAgentManager := subagents.NewManager()

	discMgr := discovery.NewManagerWithKeys(opts.KeyStore, logger)
	discCtx := context.Background()
	discMgr.Start(discCtx)

	if err := toolRegistry.Register(subagents.NewSpawnAgentTool(subAgentManager, toolRegistry)); err != nil {
		logger.Warn("Failed to register SpawnAgentTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewCheckAgentStatusTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register CheckAgentStatusTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewListAgentsTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register ListAgentsTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewCollectAgentTool(subAgentManager)); err != nil {
		logger.Warn("Failed to register CollectAgentTool", "error", err)
	}
	if err := toolRegistry.Register(subagents.NewSpawnSubAgentTool(subAgentManager, toolRegistry, discMgr)); err != nil {
		logger.Warn("Failed to register SpawnSubAgentTool", "error", err)
	}

	if err := toolRegistry.LoadDynamicTools(opts.ConfigDir); err != nil {
		logger.Warn("Failed to load dynamic tools", "error", err)
	}

	schedStore, err := scheduler.NewStore(opts.ConfigDir)
	if err != nil {
		logger.Warn("Failed to init scheduler store", "err", err)
	}
	var sched *scheduler.Scheduler
	if schedStore != nil {
		sched = scheduler.NewScheduler(schedStore, nil, logger)
	}

	userCmdLoader, err := usercommands.NewLoader(opts.ConfigDir)
	if err != nil {
		logger.Warn("Failed to init user command loader", "err", err)
		userCmdLoader = nil
	}

	if sched != nil {
		toolRegistry.SetScheduler(sched)
	}
	if userCmdLoader != nil {
		toolRegistry.SetUserCmdLoader(userCmdLoader)
	}

	logger.Info("Tool system initialized", "tool_count", len(toolRegistry.List()))

	cleanup := func() {
		senseTracer.Close()
		if auditDB != nil {
			auditDB.Close()
		}
		auditPruneCancel()
	}

	return &ToolSetup{
		ProcessManager:      processManager,
		ToolRegistry:        toolRegistry,
		SenseInputSanitizer: senseInputSanitizer,
		SenseTracer:         senseTracer,
		ResearchEngine:      researchEngine,
		ResearchGateway:     researchGateway,
		SubAgentManager:     subAgentManager,
		DiscoveryManager:    discMgr,
		SchedulerStore:      schedStore,
		Scheduler:           sched,
		UserCommandLoader:   userCmdLoader,
		Cleanup:             cleanup,
	}, nil
}
