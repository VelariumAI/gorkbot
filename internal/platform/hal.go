package platform

// hal.go — Universal Hardware Abstraction Layer (UnifiedBridge)
//
// Implements SENSE's platform-agnostic UnifiedBridge concept for Gorkbot.
// Responsibilities:
//   1. Detect the current execution environment (Termux on Android, Embedded Device, Windows, Linux, macOS).
//   2. Read system resource levels (RAM, storage) so the orchestrator can
//      adapt its behaviour on memory-constrained devices.
//   3. Expose a "5 MB RAM-spill toggle" that disables memory-intensive
//      operations (e.g., full-history compression) when free RAM falls below
//      a configurable threshold.
//
// All reads are non-fatal — the HAL gracefully degrades to "unknown" when
// a platform-specific API is unavailable.

import (
	"bufio"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// HALProfile describes the hardware and environment detected at startup.
type HALProfile struct {
	// Environment
	Platform string // "termux", "sbc", "windows", "linux", "darwin", "unknown"
	IsTermux bool
	IsSBC    bool // Raspberry Pi, Orange Pi, Rock Pi, etc.
	IsWSL    bool // Windows Subsystem for Linux

	// Resource thresholds
	TotalRAMMB     int64
	FreeRAMMB      int64
	RAMSpillActive bool // true when free RAM < RAMSpillThresholdMB
	CPUCores       int
	CPUScore       int // Micro-benchmark result (lower is faster)

	// Configuration
	RAMSpillThresholdMB int64 // default: 256 MB (practical floor for API-only work)
}

// DefaultRAMSpillThresholdMB is the free-RAM floor below which heavy in-memory
// operations are disabled.  256 MB is a conservative value that covers typical
// Termux sessions on mid-range Android devices.
const DefaultRAMSpillThresholdMB int64 = 256

// ProbeHAL detects the current hardware profile.
// Errors are logged (not returned) so the caller always gets a usable profile.
func ProbeHAL(logger *slog.Logger) HALProfile {
	p := HALProfile{
		RAMSpillThresholdMB: DefaultRAMSpillThresholdMB,
		CPUCores:            runtime.NumCPU(),
		CPUScore:            runQuickBenchmark(),
	}

	goos := runtime.GOOS

	// ── Environment detection ─────────────────────────────────────────────────
	if os.Getenv("TERMUX_VERSION") != "" {
		p.IsTermux = true
		p.Platform = "termux"
	} else if _, err := os.Stat("/data/data/com.termux/files/usr/bin/login"); err == nil {
		p.IsTermux = true
		p.Platform = "termux"
	} else if goos == "windows" {
		p.Platform = "windows"
	} else if goos == "darwin" {
		p.Platform = "darwin"
	} else if goos == "linux" {
		p.Platform = "linux"
		// Check for SBC (Raspberry Pi, Orange Pi, Rock Pi, etc.)
		p.IsSBC = detectSBC()
		if p.IsSBC {
			p.Platform = "sbc"
		}
		// Check for WSL
		p.IsWSL = detectWSL()
	} else {
		p.Platform = "unknown"
	}

	// ── RAM probing ───────────────────────────────────────────────────────────
	total, free, err := readMemInfoMB()
	if err != nil {
		if logger != nil {
			logger.Debug("HAL: could not read RAM info", "error", err)
		}
	} else {
		p.TotalRAMMB = total
		p.FreeRAMMB = free
		p.RAMSpillActive = free < p.RAMSpillThresholdMB
	}

	if logger != nil {
		logger.Info("HAL profile",
			"platform", p.Platform,
			"total_ram_mb", p.TotalRAMMB,
			"free_ram_mb", p.FreeRAMMB,
			"ram_spill", p.RAMSpillActive,
		)
	}
	return p
}

// Refresh re-reads only the dynamic metrics (RAM) without re-detecting the
// static environment.  Call periodically to keep RAMSpillActive current.
func (p *HALProfile) Refresh() {
	total, free, err := readMemInfoMB()
	if err != nil {
		return
	}
	p.TotalRAMMB = total
	p.FreeRAMMB = free
	p.RAMSpillActive = free < p.RAMSpillThresholdMB
}

// AllowHeavyOperation returns true when the device has sufficient RAM to run
// memory-intensive operations (e.g., full context compression, embeddings).
func (p *HALProfile) AllowHeavyOperation() bool {
	if p.TotalRAMMB == 0 {
		// Cannot determine RAM → be conservative on Termux/SBC, permissive elsewhere.
		return !p.IsTermux && !p.IsSBC
	}
	return !p.RAMSpillActive
}

// PlatformSummary returns a one-line human-readable description.
func (p *HALProfile) PlatformSummary() string {
	suffix := ""
	if p.TotalRAMMB > 0 {
		suffix = " | RAM " + itoa(int(p.FreeRAMMB)) + "/" + itoa(int(p.TotalRAMMB)) + " MB free"
	}
	if p.RAMSpillActive {
		suffix += " [RAM-SPILL ACTIVE]"
	}
	return p.Platform + suffix
}

// ─── Platform-specific helpers ────────────────────────────────────────────────

// detectSBC checks /proc/cpuinfo for common SBC hardware strings.
func detectSBC() bool {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return false
	}
	defer f.Close()

	sbcKeywords := []string{
		"raspberry pi",
		"bcm2835", "bcm2836", "bcm2837", "bcm2711", "bcm2712",
		"allwinner", "rockchip", "amlogic",
		"orange pi", "rock pi", "odroid",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lower := strings.ToLower(scanner.Text())
		for _, kw := range sbcKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// detectWSL checks the kernel version string for "microsoft".
func detectWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// readMemInfoMB reads /proc/meminfo and returns (totalMB, freeMB, error).
// It is used on Linux/Android (Termux) and SBC platforms.
func readMemInfoMB() (int64, int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		// Windows / macOS — we don't have /proc/meminfo.
		return 0, 0, err
	}
	defer f.Close()

	var total, free, buffers, cached int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val / 1024
		case "MemFree:":
			free = val / 1024
		case "Buffers:":
			buffers = val / 1024
		case "Cached:":
			cached = val / 1024
		}
	}
	// "usable free" = MemFree + Buffers + Cached (Linux memory accounting).
	usableFree := free + buffers + cached
	return total, usableFree, nil
}

func runQuickBenchmark() int {
	start := time.Now()
	var dummy uint64
	for i := 0; i < 5000000; i++ {
		dummy += uint64(i * i)
	}
	return int(time.Since(start).Microseconds())
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
