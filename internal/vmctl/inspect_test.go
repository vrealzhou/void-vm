package vmctl

import "testing"

func TestParseGuestMetricsSample(t *testing.T) {
	output := "cpu 100 5 20 400 10 0 2 0 0 0\nmem_total 8192000\nmem_available 2048000\n"

	sample, err := parseGuestMetricsSample(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sample.CPUTotalTicks != 537 {
		t.Fatalf("unexpected total ticks: %d", sample.CPUTotalTicks)
	}
	if sample.CPUIdleTicks != 410 {
		t.Fatalf("unexpected idle ticks: %d", sample.CPUIdleTicks)
	}
	if sample.MemTotalMiB != 8000 {
		t.Fatalf("unexpected total memory: %d", sample.MemTotalMiB)
	}
	if sample.MemUsedMiB != 6000 {
		t.Fatalf("unexpected used memory: %d", sample.MemUsedMiB)
	}
}

func TestCalculateGuestMetrics(t *testing.T) {
	previous := &GuestMetricsSample{
		CPUTotalTicks: 1000,
		CPUIdleTicks:  400,
	}
	current := GuestMetricsSample{
		CPUTotalTicks: 1200,
		CPUIdleTicks:  450,
		MemTotalMiB:   8000,
		MemUsedMiB:    2000,
	}

	metrics := CalculateGuestMetrics(current, previous)
	if !metrics.Available || !metrics.SSHReady {
		t.Fatal("expected metrics to be available")
	}
	if !metrics.HasCPUPercent {
		t.Fatal("expected cpu percentage")
	}
	if metrics.CPUPercent != 75 {
		t.Fatalf("unexpected cpu percent: %.2f", metrics.CPUPercent)
	}
	if metrics.MemUsedPercent != 25 {
		t.Fatalf("unexpected memory percent: %.2f", metrics.MemUsedPercent)
	}
}
