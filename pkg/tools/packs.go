package tools

import (
	"os"
	"strings"
)

// ToolPacks organizes all system tools into opt-in tiers to prevent context bloat.
func GetToolPacks() map[string][]Tool {
	return map[string][]Tool{
		"core": {
			NewBashTool(),
			NewStructuredBashTool(), // SENSE Module 4 — UPE: heuristic-parsed structured output
			NewReadFileTool(),
			NewWriteFileTool(),
			NewEditFileTool(),
			NewMultiEditFileTool(),
			NewListDirectoryTool(),
			NewSearchFilesTool(),
			NewGrepContentTool(),
			NewFileInfoTool(),
			NewDeleteFileTool(),
			NewContextStatsTool(),
			NewGorkbotStatusTool(),
			NewPythonSandboxTool(),
		},
		"dev": {
			NewGitStatusTool(), NewGitDiffTool(), NewGitLogTool(), NewGitCommitTool(),
			NewGitPushTool(), NewGitPullTool(), NewGitBlameAnalyzeTool(),
			NewCreateWorktreeTool(), NewListWorktreesTool(), NewRemoveWorktreeTool(), NewIntegrateWorktreeTool(),
			NewDockerManagerTool(), NewK8sKubectlTool(), NewAwsS3SyncTool(), NewNgrokTunnelTool(), NewCiTriggerTool(),
			NewCodeExecTool(), NewRebuildTool(), NewCode2WorldTool(), NewASTGrepTool(),
			NewReadFileHashedTool(), NewEditFileHashedTool(),
		},
		"web": {
			NewWebFetchTool(), NewHttpRequestTool(), NewCheckPortTool(), NewDownloadFileTool(), NewXPullTool(),
			NewWebSearchTool(), NewWebReaderTool(),
			NewScraplingFetchTool(), NewScraplingStealthTool(), NewScraplingDynamicTool(), NewScraplingExtractTool(), NewScraplingSearchTool(),
		},
		"sec": {
			NewNmapScanTool(), NewPacketCaptureTool(), NewWifiAnalyzerTool(), NewShodanQueryTool(), NewMetasploitRpcTool(), NewSslValidatorTool(),
			NewMasscanRunTool(), NewDnsEnumTool(), NewArpScanRunTool(), NewTracerouteRunTool(), NewNiktoScanTool(), NewGobusterScanTool(),
			NewFfufRunTool(), NewSqlmapScanTool(), NewWafw00fRunTool(), NewHttpHeaderAuditTool(), NewJwtDecodeTool(), NewHydraRunTool(),
			NewHashcatRunTool(), NewJohnRunTool(), NewHashIdentifyTool(), NewSearchsploitQueryTool(), NewCveLookupTool(), NewEnum4linuxRunTool(),
			NewSmbmapRunTool(), NewSuidCheckTool(), NewSudoCheckTool(), NewLinpeasRunTool(), NewStringsAnalyzeTool(), NewHexdumpFileTool(),
			NewNetstatAnalysisTool(), NewSubfinderRunTool(), NewNucleiScanTool(), NewTotpGenerateTool(),
			NewNetworkScanTool(), NewSocketConnectTool(), NewNetworkEscapeProxyTool(),
			NewBurpSuiteScanTool(), NewImpacketAttackTool(), NewTsharkCaptureTool(),
		},
		"media": {
			NewImageProcessTool(), NewMediaConvertTool(), NewFfmpegProTool(), NewAudioTranscribeTool(), NewTtsGenerateTool(),
			NewImageOcrBatchTool(), NewVideoSummarizeTool(), NewMemeGeneratorTool(),
			NewDOCXTool(), NewXLSXTool(), NewPDFTool(), NewPPTXTool(),
			NewImageResizeTool(), NewVideoConvertTool(),
		},
		"data": {
			NewCsvPivotTool(), NewPlotGenerateTool(), NewArxivSearchTool(), NewWebArchiveTool(), NewWhoisLookupTool(), NewJupyterTool(),
			NewAIImageGenerateTool(), NewAISummarizeAudioTool(), NewMLModelRunTool(),
			NewSqliteQueryTool(), NewPostgresConnectTool(),
			NewDBQueryTool(), NewDBMigrateTool(),
		},
		"sys": {
			NewPrivilegedExecTool(), // SENSE Module 2 — EAL: auto-escalation router (root/su/sudo)
			NewListProcessesTool(), NewKillProcessTool(), NewEnvVarTool(), NewSystemInfoTool(), NewDiskUsageTool(),
			NewCronManagerTool(), NewBackupRestoreTool(), NewSystemMonitorTool(), NewPkgInstallTool(),
			NewAdbScreenshotTool(), NewAdbShellTool(), NewAppCatalogTool(), NewAppControlTool(), NewAppStatusTool(),
			NewScreenCaptureTool(), NewScreenshotTool(), NewScreenrecordTool(), NewCaptureScreenHackTool(), NewUiDumpTool(),
			NewDeviceInfoTool(), NewContextStateTool(), NewKillAppTool(), NewLaunchAppTool(),
			NewManageDepsTool(), NewTermuxControlTool(), NewSaveStateTool(), NewStartHealthMonitorTool(),
			NewBrowserScrapeTool(), NewBrowserControlTool(), NewSensorReadTool(), NewNotificationSendTool(),
			NewIntentBroadcastTool(), NewLogcatDumpTool(), NewClipboardManagerTool(), NewNotificationListenerTool(),
			NewAccessibilityQueryTool(), NewApkDecompileTool(), NewSqliteExplorerTool(), NewTermuxApiBridgeTool(),
			NewTermuxSensorTool(), NewTermuxLocationTool(),
		},
		"vision": {
			NewVisionInstallTool(), NewADBSetupTool(), NewVisionScreenTool(), NewVisionCaptureOnlyTool(),
			NewVisionFileTool(), NewVisionOCRTool(), NewVisionFindTool(), NewVisionWatchTool(),
			NewVisionCaptureTool(), NewVisionAnalyzeTool(), NewLiveVisionTool(), NewScreenAnalyzeTool(),
			NewFrontendDesignTool(),
		},
		"agent": {
			NewCreateToolTool(), NewModifyToolTool(), NewListToolsTool(), NewToolInfoTool(),
			NewTodoWriteTool(), NewTodoReadTool(), NewCompleteTool(), NewConsultationTool(),
			NewRecordEngramTool(), NewSenseDiscoveryTool(), NewSenseCheckTool(), NewSenseEvolveTool(), NewSenseSanitizeTool(),
			NewRecordFactTool(), NewRecordUserPrefTool(), NewReadBrainTool(), NewForgetFactTool(),
			NewQueryRoutingStatsTool(), NewQueryHeuristicsTool(), NewQueryMemoryStateTool(), NewQuerySystemStateTool(), NewQueryAuditLogTool(),
			NewAddGoalTool(), NewCloseGoalTool(), NewListGoalsTool(), NewReportFindingTool(), NewRunPipelineTool(),
			NewScheduleTaskTool(), NewListScheduledTasksTool(), NewCancelScheduledTaskTool(), NewPauseResumeScheduledTaskTool(), NewDefineCommandTool(),
			NewDocParserTool(),
		},
		"comm": {
			NewSendEmailTool(), NewSlackNotifyTool(),
			NewCalendarManageTool(), NewEmailClientTool(), NewContactSyncTool(), NewSmartHomeApiTool(),
		},
	}
}

// GetActivePacks returns the list of pack names to load based on GORKBOT_TOOL_PACKS,
// defaulting to a sensible lean set if empty.
func GetActivePacks() []string {
	env := os.Getenv("GORKBOT_TOOL_PACKS")
	if env == "" {
		return []string{"core", "dev", "web", "sys", "agent", "data", "media", "comm"} // Load the new categories
	}
	if env == "ALL" {
		return []string{"core", "dev", "web", "sec", "media", "data", "sys", "vision", "agent", "comm"}
	}
	return strings.Split(env, ",")
}
