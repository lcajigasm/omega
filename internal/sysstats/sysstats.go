// Package sysstats measures how busy the machine is: the processor, the
// memory, the disks and the network. The screens of the terminal interface
// show it on every frame, so it is measured in the background, once a
// second, and read without waiting.
package sysstats

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Interval is how often the machine is measured.
const Interval = time.Second

// Disk is one volume: where it is mounted and how full it is.
type Disk struct {
	Mount       string
	Used, Total uint64
}

// UsedShare is the share of the volume in use, from 0 to 1.
func (d Disk) UsedShare() float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(d.Used) / float64(d.Total)
}

// Sample is the machine at one moment. A part that could not be measured
// is false in its Known field and zero otherwise.
type Sample struct {
	At time.Time

	CPU      float64 // share of the processor in use, from 0 to 1
	CPUKnown bool    // false until two readings a second apart exist
	Cores    int

	MemUsed, MemTotal uint64
	MemKnown          bool

	Disks []Disk // the system volume first

	RxRate, TxRate float64 // bytes a second received and sent
	NetKnown       bool    // false until two readings exist
}

// MemShare is the share of the memory in use, from 0 to 1.
func (s Sample) MemShare() float64 {
	if s.MemTotal == 0 {
		return 0
	}
	return float64(s.MemUsed) / float64(s.MemTotal)
}

// reader is what the sampler reads the machine with; tests replace it.
type reader interface {
	cpuTimes() (busy, total float64, ok bool)
	memory() (used, total uint64, ok bool)
	disks() []Disk
	netBytes() (rx, tx uint64, ok bool)
}

// Sampler measures the machine every Interval while someone watches.
type Sampler struct {
	r      reader
	latest atomic.Pointer[Sample]

	mu      sync.Mutex
	users   int
	stop    context.CancelFunc
	done    chan struct{}
	prevCPU [2]float64 // busy and total of the last reading
	cpuOK   bool
	prevNet [2]uint64
	netAt   time.Time
	netOK   bool
	disksAt time.Time
	disks   []Disk
}

var shared = &Sampler{r: system{}}

// Watch starts measuring the machine in the background, if nothing else is,
// and returns the function that says it is no longer needed. The first
// sample is ready when Watch returns.
func Watch() (release func()) { return shared.Watch() }

// Latest is the last sample measured; the zero Sample when nothing is
// measuring the machine.
func Latest() Sample { return shared.Latest() }

// Watch is the package's Watch on this sampler.
func (s *Sampler) Watch() func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users++
	if s.users == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		s.stop, s.done = cancel, make(chan struct{})
		s.measure(time.Now())
		go s.loop(ctx, s.done)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.users--
			var done chan struct{}
			if s.users == 0 {
				s.stop()
				done = s.done
			}
			s.mu.Unlock()
			if done != nil {
				<-done
			}
		})
	}
}

// Latest is the package's Latest on this sampler.
func (s *Sampler) Latest() Sample {
	if p := s.latest.Load(); p != nil {
		return *p
	}
	return Sample{}
}

func (s *Sampler) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			s.measure(now)
			s.mu.Unlock()
		}
	}
}

// disksEvery is how often the list of volumes is read again: a disk is
// plugged in far less often than the processor changes.
const disksEvery = 5 * time.Second

// measure takes one sample. The caller holds s.mu.
func (s *Sampler) measure(now time.Time) {
	out := Sample{At: now, Cores: runtime.NumCPU()}
	if busy, total, ok := s.r.cpuTimes(); ok {
		if s.cpuOK && total > s.prevCPU[1] {
			out.CPU = clamp((busy - s.prevCPU[0]) / (total - s.prevCPU[1]))
			out.CPUKnown = true
		} else if prev := s.Latest(); prev.CPUKnown {
			out.CPU, out.CPUKnown = prev.CPU, true // too soon to tell: the last figure stays
		}
		s.prevCPU, s.cpuOK = [2]float64{busy, total}, true
	}
	out.MemUsed, out.MemTotal, out.MemKnown = s.r.memory()
	if s.disks == nil || now.Sub(s.disksAt) >= disksEvery {
		s.disks, s.disksAt = s.r.disks(), now
		if s.disks == nil {
			s.disks = []Disk{}
		}
	}
	out.Disks = s.disks
	if rx, tx, ok := s.r.netBytes(); ok {
		if s.netOK {
			if secs := now.Sub(s.netAt).Seconds(); secs > 0 {
				out.RxRate, out.TxRate, out.NetKnown = rate(rx, s.prevNet[0], secs), rate(tx, s.prevNet[1], secs), true
			}
		}
		s.prevNet, s.netAt, s.netOK = [2]uint64{rx, tx}, now, true
	}
	s.latest.Store(&out)
}

// rate is bytes a second from two counter readings; a counter that went
// back (an interface that came and went) counts as nothing moved.
func rate(now, before uint64, secs float64) float64 {
	if now < before {
		return 0
	}
	return float64(now-before) / secs
}

func clamp(v float64) float64 { return min(1, max(0, v)) }

// system reads the real machine.
type system struct{}

func (system) cpuTimes() (busy, total float64, ok bool) {
	ts, err := cpu.Times(false)
	if err != nil || len(ts) == 0 {
		return 0, 0, false
	}
	t := ts[0]
	idle := t.Idle + t.Iowait
	total = t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	return total - idle, total, total > 0
}

func (system) memory() (used, total uint64, ok bool) {
	v, err := mem.VirtualMemory()
	if err != nil || v.Total == 0 {
		return 0, 0, false
	}
	// What is available counts the caches the system gives back at once:
	// the rest is what programs hold.
	used = v.Total - min(v.Available, v.Total)
	return used, v.Total, true
}

// skipFS are the file systems that are not disks a person thinks of.
var skipFS = map[string]bool{
	"devfs": true, "autofs": true, "tmpfs": true, "devtmpfs": true, "proc": true, "sysfs": true,
	"overlay": true, "squashfs": true, "nullfs": true, "cgroup": true, "cgroup2": true,
	"fuse.snapfuse": true, "ramfs": true, "efivarfs": true, "bpf": true, "tracefs": true,
	"debugfs": true, "securityfs": true, "pstore": true, "configfs": true, "fusectl": true,
	"mqueue": true, "hugetlbfs": true, "binfmt_misc": true, "nsfs": true, "rpc_pipefs": true,
}

func (system) disks() []Disk {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	type cand struct {
		Disk
		free uint64
	}
	var all []cand
	for _, p := range parts {
		if skipFS[strings.ToLower(p.Fstype)] || hidden(p.Mountpoint, p.Opts) {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		all = append(all, cand{Disk{Mount: p.Mountpoint, Used: u.Total - min(u.Free, u.Total), Total: u.Total}, u.Free})
	}
	return pickDisks(all, func(c cand) (Disk, uint64) { return c.Disk, c.free })
}

// pickDisks keeps one volume per disk: the volumes of one APFS container,
// or one disk mounted twice, have the same size and the same free space.
// The system volume goes first, then the rest by where they are mounted.
func pickDisks[T any](all []T, get func(T) (Disk, uint64)) []Disk {
	type key struct{ total, free uint64 }
	seen := map[key]int{}
	var out []Disk
	for _, c := range all {
		d, free := get(c)
		k := key{d.Total, free}
		if i, ok := seen[k]; ok {
			if systemMount(d.Mount) && !systemMount(out[i].Mount) {
				out[i] = d
			}
			continue
		}
		seen[k] = len(out)
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := systemMount(out[i].Mount), systemMount(out[j].Mount); a != b {
			return a
		}
		return out[i].Mount < out[j].Mount
	})
	return out
}

// systemMount reports whether a mount point is where the system lives.
func systemMount(m string) bool {
	return m == "/" || strings.EqualFold(strings.TrimRight(m, `\`), "C:")
}

// hidden reports whether a volume is one the system keeps for itself: on
// macOS they are mounted "nobrowse" (Preboot, VM, Recovery…); elsewhere
// they live under the system's own directories.
func hidden(mount string, opts []string) bool {
	if systemMount(mount) {
		return false
	}
	for _, o := range opts {
		if o == "nobrowse" {
			return true
		}
	}
	for _, p := range []string{"/System/Volumes/", "/boot", "/snap/", "/run/", "/var/lib/docker", "/dev/", "/proc", "/sys"} {
		if strings.HasPrefix(mount, p) {
			return true
		}
	}
	return false
}

// virtualNIC reports whether a network interface carries no traffic of its
// own: the loopback, and tunnels and bridges whose traffic the physical
// interface already counts.
func virtualNIC(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{"lo", "utun", "awdl", "llw", "gif", "stf", "anpi", "bridge", "ap1",
		"docker", "veth", "br-", "virbr", "vmnet", "vboxnet", "tun", "tap", "wg", "tailscale", "zt",
		"loopback", "isatap", "teredo"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return strings.Contains(n, "loopback") || strings.Contains(n, "pseudo")
}

func (system) netBytes() (rx, tx uint64, ok bool) {
	cs, err := net.IOCounters(true)
	if err != nil {
		return 0, 0, false
	}
	for _, c := range cs {
		if virtualNIC(c.Name) {
			continue
		}
		rx += c.BytesRecv
		tx += c.BytesSent
	}
	return rx, tx, true
}
