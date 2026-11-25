package sysstats

import (
	"math"
	"testing"
	"time"
)

type fake struct {
	busy, total float64
	rx, tx      uint64
	vols        []Disk
	diskReads   int
}

func (f *fake) cpuTimes() (float64, float64, bool) { return f.busy, f.total, true }
func (f *fake) memory() (uint64, uint64, bool)     { return 6 << 30, 16 << 30, true }
func (f *fake) disks() []Disk                      { f.diskReads++; return f.vols }
func (f *fake) netBytes() (uint64, uint64, bool)   { return f.rx, f.tx, true }

func TestMeasureRates(t *testing.T) {
	f := &fake{busy: 100, total: 1000, rx: 1000, tx: 500, vols: []Disk{{Mount: "/", Used: 1, Total: 4}}}
	s := &Sampler{r: f}
	t0 := time.Unix(1000, 0)
	s.measure(t0)
	first := s.Latest()
	if first.CPUKnown || first.NetKnown {
		t.Fatalf("the first sample cannot know the rates: %+v", first)
	}
	if !first.MemKnown || math.Abs(first.MemShare()-0.375) > 1e-9 {
		t.Fatalf("memory = %v", first.MemShare())
	}
	f.busy, f.total, f.rx, f.tx = 400, 2000, 3000, 1500
	s.measure(t0.Add(2 * time.Second))
	got := s.Latest()
	if !got.CPUKnown || math.Abs(got.CPU-0.3) > 1e-9 {
		t.Errorf("cpu = %v, want 0.3", got.CPU)
	}
	if !got.NetKnown || got.RxRate != 1000 || got.TxRate != 500 {
		t.Errorf("net = %v %v, want 1000 500", got.RxRate, got.TxRate)
	}
	if f.diskReads != 1 {
		t.Errorf("the volumes were read %d times in 2s, want once", f.diskReads)
	}
	// A counter that went back (an interface gone) is no traffic.
	f.rx = 10
	s.measure(t0.Add(3 * time.Second))
	if got := s.Latest(); got.RxRate != 0 {
		t.Errorf("rx after a reset = %v", got.RxRate)
	}
	// No time between two processor readings: the last figure stays.
	s.measure(t0.Add(3 * time.Second))
	if got := s.Latest(); !got.CPUKnown {
		t.Error("the processor figure was lost")
	}
}

func TestPickDisks(t *testing.T) {
	type c struct {
		d    Disk
		free uint64
	}
	all := []c{
		{Disk{"/System/Volumes/Data", 60, 100}, 40},
		{Disk{"/Volumes/Backup", 10, 500}, 490},
		{Disk{"/", 60, 100}, 40},
		{Disk{"/Volumes/Alpha", 1, 50}, 49},
	}
	got := pickDisks(all, func(x c) (Disk, uint64) { return x.d, x.free })
	want := []string{"/", "/Volumes/Alpha", "/Volumes/Backup"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, d := range got {
		if d.Mount != want[i] {
			t.Errorf("disk %d = %s, want %s", i, d.Mount, want[i])
		}
	}
}

func TestHiddenAndVirtual(t *testing.T) {
	if hidden("/", []string{"ro", "nobrowse"}) {
		t.Error("the system volume is never hidden")
	}
	if !hidden("/System/Volumes/VM", nil) || !hidden("/Volumes/Recovery", []string{"nobrowse"}) {
		t.Error("system volumes should be hidden")
	}
	if hidden("/Volumes/USB", []string{"rw"}) {
		t.Error("an external disk is shown")
	}
	for _, n := range []string{"lo0", "lo", "utun4", "awdl0", "docker0", "Loopback Pseudo-Interface 1"} {
		if !virtualNIC(n) {
			t.Errorf("%s should not count", n)
		}
	}
	for _, n := range []string{"en0", "eth0", "wlan0", "Wi-Fi", "Ethernet"} {
		if virtualNIC(n) {
			t.Errorf("%s should count", n)
		}
	}
}

func TestWatchShares(t *testing.T) {
	s := &Sampler{r: &fake{total: 1}}
	r1 := s.Watch()
	r2 := s.Watch()
	if s.Latest().At.IsZero() {
		t.Fatal("Watch returns with a sample ready")
	}
	r1()
	r1() // twice is once
	if s.users != 1 {
		t.Fatalf("users = %d", s.users)
	}
	r2()
	if s.users != 0 {
		t.Fatalf("users = %d", s.users)
	}
}

// The real machine: whatever it reports must make sense.
func TestSystem(t *testing.T) {
	release := Watch()
	defer release()
	s := Latest()
	if s.MemKnown && (s.MemUsed > s.MemTotal || s.MemTotal == 0) {
		t.Errorf("memory %d of %d", s.MemUsed, s.MemTotal)
	}
	for _, d := range s.Disks {
		if d.Used > d.Total {
			t.Errorf("%s: %d of %d", d.Mount, d.Used, d.Total)
		}
	}
}
