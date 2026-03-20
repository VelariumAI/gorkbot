package spark

import (
	"context"
	"time"
)

// ageMemReader is the minimal interface over *sense.AgeMem.
type ageMemReader interface {
	ShouldPrune() bool
	UsageStats() map[string]interface{}
}

// Introspector captures SPARKState by querying TII, IDL, AgeMem,
// MotivationalCore, and ResearchModule.
type Introspector struct {
	tii    *EfficiencyEngine
	idl    *ImprovementDebtLedger
	ageMem ageMemReader // nil-safe
	mc     *MotivationalCore
	rm     *ResearchModule
}

// NewIntrospector creates an Introspector. All components except tii and idl
// may be nil.
func NewIntrospector(tii *EfficiencyEngine, idl *ImprovementDebtLedger,
	ageMem ageMemReader, mc *MotivationalCore, rm *ResearchModule) *Introspector {
	return &Introspector{tii: tii, idl: idl, ageMem: ageMem, mc: mc, rm: rm}
}

// Snapshot captures the full system state at cycle start.
func (i *Introspector) Snapshot(_ context.Context) *SPARKState {
	s := &SPARKState{
		CapturedAt:       time.Now(),
		TIISnapshot:      i.tii.Snapshot(),
		IDLSnapshot:      i.idl.Top(10),
		ActiveDirectives: i.idl.Len(),
		SubsystemHealth:  i.checkHealth(),
	}
	if i.mc != nil {
		s.DriveScore = i.mc.DriveScore()
	}
	if i.rm != nil {
		s.TopObjective = i.rm.Top()
	}
	if i.ageMem != nil {
		if i.ageMem.ShouldPrune() {
			s.MemoryPressure = 0.9
		} else if stats := i.ageMem.UsageStats(); stats != nil {
			if pct, ok := stats["stm_usage_pct"].(float64); ok {
				s.MemoryPressure = pct / 100.0
			}
		}
	}
	return s
}

// checkHealth evaluates all tracked subsystems and returns status strings.
func (i *Introspector) checkHealth() map[string]string {
	h := map[string]string{}

	// TII: warn if any tool has >10 calls and <50% success.
	h["tii"] = "ok"
	for _, e := range i.tii.Snapshot() {
		if e.Invocations > 10 && e.SuccessRate < 0.5 {
			h["tii"] = "warn"
			break
		}
	}

	// IDL: warn at 80% capacity, error at 100%.
	n, max := i.idl.Len(), i.idl.MaxSize()
	switch {
	case n >= max:
		h["idl"] = "error"
	case n*5 >= max*4:
		h["idl"] = "warn"
	default:
		h["idl"] = "ok"
	}

	// Memory pressure.
	h["memory"] = "ok"
	if i.ageMem != nil && i.ageMem.ShouldPrune() {
		h["memory"] = "warn"
	}

	// Motivational core.
	if i.mc == nil {
		h["motivational_core"] = "disabled"
	} else {
		if i.mc.DriveScore() < 0.2 {
			h["motivational_core"] = "warn"
		} else {
			h["motivational_core"] = "ok"
		}
	}

	// Research module.
	if i.rm == nil {
		h["research"] = "disabled"
	} else {
		h["research"] = "ok"
	}

	return h
}
