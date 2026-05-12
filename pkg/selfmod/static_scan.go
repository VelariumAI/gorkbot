package selfmod

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

type StaticScanResult struct {
	Allowed    bool
	ReasonCode string
	Issues     []string
}

var forbiddenImports = map[string]string{
	"unsafe":  REASON_DYNAMIC_IMPORT_FORBIDDEN,
	"syscall": REASON_DYNAMIC_IMPORT_FORBIDDEN,
	"os/exec": REASON_DYNAMIC_EXEC_FORBIDDEN,
	"plugin":  REASON_DYNAMIC_IMPORT_FORBIDDEN,
}

func StaticScanGoSource(source string, caps []DynamicCapability) StaticScanResult {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "generated.go", source, parser.AllErrors)
	if err != nil {
		return StaticScanResult{Allowed: false, ReasonCode: REASON_DYNAMIC_STATIC_SCAN_FAILED, Issues: []string{err.Error()}}
	}

	capSet := make(map[DynamicCapability]bool, len(caps))
	for _, c := range caps {
		capSet[normalizeCapability(string(c))] = true
	}

	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if reason, blocked := forbiddenImports[path]; blocked {
			return StaticScanResult{Allowed: false, ReasonCode: reason, Issues: []string{"forbidden import: " + path}}
		}
		if (path == "net" || path == "net/http" || path == "net/url") && !capSet[CapabilityNetworkFetch] {
			return StaticScanResult{Allowed: false, ReasonCode: REASON_DYNAMIC_NETWORK_FORBIDDEN, Issues: []string{"missing capability for import: " + path}}
		}
	}

	var firstIssue string
	var firstReason string

	ast.Inspect(file, func(n ast.Node) bool {
		if firstIssue != "" {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil && x.Name != nil && x.Name.Name == "init" {
				firstReason = REASON_DYNAMIC_INIT_FORBIDDEN
				firstIssue = "init function is forbidden"
				return false
			}
		case *ast.CallExpr:
			if reason, issue := classifyCall(x, capSet); reason != "" {
				firstReason = reason
				firstIssue = issue
				return false
			}
		}
		return true
	})

	if firstIssue != "" {
		return StaticScanResult{Allowed: false, ReasonCode: firstReason, Issues: []string{firstIssue}}
	}
	return StaticScanResult{Allowed: true}
}

func classifyCall(call *ast.CallExpr, caps map[DynamicCapability]bool) (reason string, issue string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}

	n := fmt.Sprintf("%s.%s", pkg.Name, sel.Sel.Name)
	switch n {
	case "exec.Command", "exec.CommandContext":
		return REASON_DYNAMIC_EXEC_FORBIDDEN, "process execution is forbidden"
	case "http.NewRequest", "http.NewRequestWithContext", "http.DefaultClient", "http.Get", "http.Post":
		if !caps[CapabilityNetworkFetch] {
			return REASON_DYNAMIC_NETWORK_FORBIDDEN, "network call without dynamic.network.fetch"
		}
	case "os.Remove", "os.RemoveAll":
		if !caps[CapabilityDeleteFile] {
			return REASON_DYNAMIC_CAPABILITY_FORBIDDEN, "file delete sink requires dynamic.delete_file"
		}
	case "os.WriteFile", "os.Create":
		if !caps[CapabilityWriteFile] {
			return REASON_DYNAMIC_CAPABILITY_FORBIDDEN, "file write sink requires dynamic.write_file"
		}
	case "os.Open", "os.ReadFile":
		if !caps[CapabilityReadFile] {
			return REASON_DYNAMIC_CAPABILITY_FORBIDDEN, "file read sink requires dynamic.read_file"
		}
	case "os.Getenv", "os.LookupEnv":
		return REASON_DYNAMIC_CREDENTIAL_ACCESS_FORBIDDEN, "environment variable access is forbidden in generated code"
	}
	return "", ""
}
