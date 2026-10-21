package base

import "testing"

func TestBudgets(t *testing.T) {
	tests := []struct{ max, text, provider int }{
		{102400, 101888, 100352},
		{1024, 512, 768},
		{4096, 3584, 3072},
	}
	for _, tc := range tests {
		if got := TextBudget(tc.max); got != tc.text {
			t.Errorf("TextBudget(%d)=%d want %d", tc.max, got, tc.text)
		}
		if got := ProviderMaxBytes(tc.max); got != tc.provider {
			t.Errorf("ProviderMaxBytes(%d)=%d want %d", tc.max, got, tc.provider)
		}
	}
	if TextBudget(100) != 512 {
		t.Error("minimum")
	}
}
