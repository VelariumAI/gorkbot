package selfmod

import "testing"

func TestStaticScanPureGoAllowed(t *testing.T) {
	src := "package tools\nimport (\"fmt\")\nfunc Run() string { return fmt.Sprint(\"ok\") }"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if !res.Allowed {
		t.Fatalf("expected pure source allowed: %+v", res)
	}
}

func TestStaticScanInvalidGoBlocked(t *testing.T) {
	res := StaticScanGoSource("package", []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_STATIC_SCAN_FAILED {
		t.Fatalf("expected invalid go blocked, got %+v", res)
	}
}

func TestStaticScanUnsafeBlocked(t *testing.T) {
	src := "package tools\nimport \"unsafe\"\nfunc Run(){_ = unsafe.Sizeof(1)}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_IMPORT_FORBIDDEN {
		t.Fatalf("expected unsafe import blocked, got %+v", res)
	}
}

func TestStaticScanExecBlocked(t *testing.T) {
	src := "package tools\nimport \"os/exec\"\nfunc Run(){exec.Command(\"sh\",\"-c\",\"echo hi\")}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_EXEC_FORBIDDEN {
		t.Fatalf("expected os/exec blocked, got %+v", res)
	}
}

func TestStaticScanSyscallBlocked(t *testing.T) {
	src := "package tools\nimport \"syscall\"\nfunc Run(){_ = syscall.Getpid()}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_IMPORT_FORBIDDEN {
		t.Fatalf("expected syscall blocked, got %+v", res)
	}
}

func TestStaticScanInitBlocked(t *testing.T) {
	src := "package tools\nfunc init(){}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_INIT_FORBIDDEN {
		t.Fatalf("expected init blocked, got %+v", res)
	}
}

func TestStaticScanNetworkRequiresCapability(t *testing.T) {
	src := "package tools\nimport \"net/http\"\nfunc Run(){_,_ = http.NewRequest(\"GET\",\"https://x\",nil)}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_NETWORK_FORBIDDEN {
		t.Fatalf("expected network without capability blocked, got %+v", res)
	}
}

func TestStaticScanDeleteRequiresCapability(t *testing.T) {
	src := "package tools\nimport \"os\"\nfunc Run(){_ = os.Remove(\"a\")}"
	res := StaticScanGoSource(src, []DynamicCapability{CapabilitySkillStage})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected delete sink blocked, got %+v", res)
	}
}
