package memory

import (
	"testing"
	"time"
)

func TestComputeRecency_PositiveForRecentFact(t *testing.T) {
	as := NewAdvancedSearcher(nil, nil, nil)
	fact := &Fact{
		LastConfirmed: time.Now().Unix(),
	}
	recency := as.computeRecency(fact)
	if recency <= 0.5 {
		t.Errorf("expected recency > 0.5 for recent fact, got %v", recency)
	}
}

func TestComputeRecency_NearZeroForOldFact(t *testing.T) {
	as := NewAdvancedSearcher(nil, nil, nil)
	fact := &Fact{
		LastConfirmed: time.Now().Unix() - 365*24*3600,
	}
	recency := as.computeRecency(fact)
	if recency >= 0.1 {
		t.Errorf("expected recency < 0.1 for old fact, got %v", recency)
	}
}

func TestComputeRecency_RecentBeatOld(t *testing.T) {
	as := NewAdvancedSearcher(nil, nil, nil)
	now := time.Now().Unix()
	
	recent := &Fact{LastConfirmed: now}
	old := &Fact{LastConfirmed: now - 100*24*3600}
	
	rRecent := as.computeRecency(recent)
	rOld := as.computeRecency(old)
	
	if rRecent <= rOld {
		t.Errorf("expected recent (%v) to beat old (%v)", rRecent, rOld)
	}
}

func TestRankByRecency(t *testing.T) {
	as := NewAdvancedSearcher(nil, nil, nil)
	now := time.Now().Unix()
	
	f1 := &Fact{LastConfirmed: now}
	f2 := &Fact{LastConfirmed: now - 3*24*3600}
	f3 := &Fact{LastConfirmed: now - 15*24*3600}
	f4 := &Fact{LastConfirmed: now - 60*24*3600}
	
	if rank := as.rankByRecency(f1); rank != 1 {
		t.Errorf("expected rank 1, got %d", rank)
	}
	if rank := as.rankByRecency(f2); rank != 2 {
		t.Errorf("expected rank 2, got %d", rank)
	}
	if rank := as.rankByRecency(f3); rank != 3 {
		t.Errorf("expected rank 3, got %d", rank)
	}
	if rank := as.rankByRecency(f4); rank != 4 {
		t.Errorf("expected rank 4, got %d", rank)
	}
}
