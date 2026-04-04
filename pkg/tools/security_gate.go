package tools

// securityToolNames is the definitive set of security/penetration testing tool names.
// These tools require explicit security mode enablement to prevent accidental exposure
// of powerful attack capabilities to untrusted AI agents.
var securityToolNames = map[string]bool{
	"nmap_scan":            true,
	"packet_capture":       true,
	"wifi_analyzer":        true,
	"shodan_query":         true,
	"metasploit_rpc":       true,
	"ssl_validator":        true,
	"masscan_run":          true,
	"dns_enum":             true,
	"arp_scan_run":         true,
	"traceroute_run":       true,
	"nikto_scan":           true,
	"gobuster_scan":        true,
	"ffuf_run":             true,
	"sqlmap_scan":          true,
	"wafw00f_run":          true,
	"http_header_audit":    true,
	"jwt_decode":           true,
	"hydra_run":            true,
	"hashcat_run":          true,
	"john_run":             true,
	"hash_identify":        true,
	"searchsploit_query":   true,
	"cve_lookup":           true,
	"enum4linux_run":       true,
	"smbmap_run":           true,
	"suid_check":           true,
	"sudo_check":           true,
	"linpeas_run":          true,
	"strings_analyze":      true,
	"hexdump_file":         true,
	"netstat_analysis":     true,
	"subfinder_run":        true,
	"nuclei_scan":          true,
	"totp_generate":        true,
	"network_scan":         true,
	"socket_connect":       true,
	"network_escape_proxy": true,
	"burp_suite_scan":      true,
	"impacket_attack":      true,
	"tshark_capture":       true,
}

// isSecurityTool returns true if the given tool name is a security/penetration testing tool.
func isSecurityTool(name string) bool {
	return securityToolNames[name]
}
