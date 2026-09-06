package diskspace

import "testing"

func TestQuery_ReturnsPlausibleUsageForTempDir(t *testing.T) {
	u, err := Query(t.TempDir())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if u.Total == 0 {
		t.Error("Total = 0, want a real filesystem size")
	}
	if u.Free > u.Total {
		t.Errorf("Free (%d) > Total (%d)", u.Free, u.Total)
	}
}
