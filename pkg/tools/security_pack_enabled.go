//go:build with_security

package tools

// getSecurityPack returns all security/penetration testing tools.
// This build is only included when compiled with -tags with-security.
func getSecurityPack() []Tool {
	return []Tool{
		NewNmapScanTool(), NewPacketCaptureTool(), NewWifiAnalyzerTool(), NewShodanQueryTool(), NewMetasploitRpcTool(), NewSslValidatorTool(),
		NewMasscanRunTool(), NewDnsEnumTool(), NewArpScanRunTool(), NewTracerouteRunTool(), NewNiktoScanTool(), NewGobusterScanTool(),
		NewFfufRunTool(), NewSqlmapScanTool(), NewWafw00fRunTool(), NewHttpHeaderAuditTool(), NewJwtDecodeTool(), NewHydraRunTool(),
		NewHashcatRunTool(), NewJohnRunTool(), NewHashIdentifyTool(), NewSearchsploitQueryTool(), NewCveLookupTool(), NewEnum4linuxRunTool(),
		NewSmbmapRunTool(), NewSuidCheckTool(), NewSudoCheckTool(), NewLinpeasRunTool(), NewStringsAnalyzeTool(), NewHexdumpFileTool(),
		NewNetstatAnalysisTool(), NewSubfinderRunTool(), NewNucleiScanTool(), NewTotpGenerateTool(),
		NewNetworkScanTool(), NewSocketConnectTool(), NewNetworkEscapeProxyTool(),
		NewBurpSuiteScanTool(), NewImpacketAttackTool(), NewTsharkCaptureTool(),
	}
}
