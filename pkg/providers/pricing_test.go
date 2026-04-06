package providers

import "testing"

func TestGetPriceExactAndPrefix(t *testing.T) {
	exact, ok := GetPrice("gpt-4o")
	if !ok {
		t.Fatal("expected exact model pricing lookup to succeed")
	}
	if exact.InputPerMTok != 2.50 {
		t.Fatalf("unexpected exact input price: %v", exact.InputPerMTok)
	}

	prefix, ok := GetPrice("gpt-4o-2025-01-01")
	if !ok {
		t.Fatal("expected prefix model pricing lookup to succeed")
	}
	if prefix.InputPerMTok != exact.InputPerMTok || prefix.OutputPerMTok != exact.OutputPerMTok {
		t.Fatalf("expected prefix price to match gpt-4o, got %+v vs %+v", prefix, exact)
	}
}

func TestGetPriceUnknownModel(t *testing.T) {
	if _, ok := GetPrice("model-that-does-not-exist"); ok {
		t.Fatal("expected unknown model pricing lookup to fail")
	}
}

func TestCostPerRequest(t *testing.T) {
	// gpt-4o = $2.50 / 1M input, $10 / 1M output
	got := CostPerRequest("gpt-4o", 1000, 500)
	want := (1000 * 2.50 / 1e6) + (500 * 10.00 / 1e6)
	if got != want {
		t.Fatalf("unexpected request cost: got %f want %f", got, want)
	}
}

func TestIsCheapModel(t *testing.T) {
	if !IsCheapModel("grok-3-mini") {
		t.Fatal("expected grok-3-mini to be cheap")
	}
	if IsCheapModel("grok-3") {
		t.Fatal("expected grok-3 to not be cheap")
	}
	if IsCheapModel("unknown-model") {
		t.Fatal("unknown model must not be treated as cheap")
	}
}
