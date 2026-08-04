package timezone

import (
	"testing"
	"time"
)

func TestPolicyStorageRoundTrip(t *testing.T) {
	original := time.Date(2026, 8, 4, 6, 50, 10, 123456789, time.FixedZone("test", 9*60*60))
	encoded := FormatStorage(original)
	if encoded != "2026-08-03T21:50:10.123456Z" {
		t.Fatalf("FormatStorage() = %q", encoded)
	}
	parsed, err := ParseStorage(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Equal(original.Truncate(time.Microsecond)) || parsed.Location() != time.UTC {
		t.Fatalf("ParseStorage() = %v", parsed)
	}
}

func TestPolicyRejectsInvalidTimezone(t *testing.T) {
	if _, err := New("Mars/Olympus_Mons"); err == nil {
		t.Fatal("expected invalid timezone error")
	}
}

func TestPolicyParseInputUsesConfiguredTimezone(t *testing.T) {
	policy, err := New("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := policy.ParseInput("2026-08-04T14:50")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatStorage(parsed); got != "2026-08-04T06:50:00.000000Z" {
		t.Fatalf("ParseInput() = %s", got)
	}

	parsed, err = policy.ParseInput("2026-08-04T14:50:00-07:00")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatStorage(parsed); got != "2026-08-04T21:50:00.000000Z" {
		t.Fatalf("offset ParseInput() = %s", got)
	}
}

func TestPolicyRejectsNonexistentDSTWallTime(t *testing.T) {
	policy, err := New("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.ParseInput("2026-03-08T02:30:00"); err == nil {
		t.Fatal("expected nonexistent DST wall time error")
	}
}

func TestPolicyChoosesEarlierRepeatedDSTInstant(t *testing.T) {
	policy, err := New("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := policy.ParseInput("2026-11-01T01:30:00")
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatStorage(parsed); got != "2026-11-01T05:30:00.000000Z" {
		t.Fatalf("ambiguous ParseInput() = %s", got)
	}
}

func TestPolicyDayRangeHandlesDSTLengths(t *testing.T) {
	policy, err := New("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		date string
		want time.Duration
	}{
		{date: "2026-03-08", want: 23 * time.Hour},
		{date: "2026-08-04", want: 24 * time.Hour},
		{date: "2026-11-01", want: 25 * time.Hour},
	}
	for _, test := range tests {
		t.Run(test.date, func(t *testing.T) {
			start, end, err := policy.DayRange(test.date)
			if err != nil {
				t.Fatal(err)
			}
			if got := end.Sub(start); got != test.want {
				t.Fatalf("DayRange() duration = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPolicyUpdateIsVisibleAtomically(t *testing.T) {
	policy, err := New("UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Update("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	if policy.Name() != "Asia/Shanghai" || policy.Location().String() != "Asia/Shanghai" {
		t.Fatalf("unexpected policy snapshot: %q %q", policy.Name(), policy.Location())
	}
}

func TestPolicySnapshotRemainsStableAfterUpdate(t *testing.T) {
	policy, err := New("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := policy.Snapshot()
	if err := policy.Update("UTC"); err != nil {
		t.Fatal(err)
	}
	if snapshot.Name() != "Asia/Shanghai" || policy.Name() != "UTC" {
		t.Fatalf("snapshot=%q policy=%q", snapshot.Name(), policy.Name())
	}
}

func TestDBTimeFailsClosed(t *testing.T) {
	var value DBTime
	if err := value.Scan(time.Now()); err == nil {
		t.Fatal("expected driver-normalized time to fail")
	}
	if err := value.Scan("2026-08-04 14:50:10"); err == nil {
		t.Fatal("expected unzoned database time to fail")
	}
	if err := value.Scan("2026-08-04T06:50:10.000000Z"); err != nil {
		t.Fatal(err)
	}
	if got := FormatStorage(value.Time); got != "2026-08-04T06:50:10.000000Z" {
		t.Fatalf("DBTime = %s", got)
	}
}
