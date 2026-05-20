package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Sampler keeps state between reads so it can compute delta-based metrics
// (CPU busy %, network throughput) which are inherently rate-based.
type Sampler struct {
	iface string

	lastCPUTotal float64
	lastCPUBusy  float64
	lastNetRx    uint64
	lastNetTx    uint64
	lastNetAt    time.Time
}

func NewSampler(iface string) *Sampler { return &Sampler{iface: iface} }

type SampleSnapshot struct {
	CPUPct        float64
	RAMPct        float64
	NetInMbps     float64
	NetOutMbps    float64
	UptimeSeconds int64
}

func (s *Sampler) Sample() SampleSnapshot {
	snap := SampleSnapshot{
		CPUPct:        s.cpuPct(),
		RAMPct:        s.ramPct(),
		UptimeSeconds: s.uptime(),
	}
	snap.NetInMbps, snap.NetOutMbps = s.netMbps()
	return snap
}

func (s *Sampler) cpuPct() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}
	// "cpu  user nice system idle iowait irq softirq steal ..."
	fields := strings.Fields(sc.Text())
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0
	}
	vals := make([]float64, 0, 8)
	for _, x := range fields[1:8] {
		v, _ := strconv.ParseFloat(x, 64)
		vals = append(vals, v)
	}
	var total float64
	for _, v := range vals {
		total += v
	}
	idle := vals[3] + vals[4] // idle + iowait
	busy := total - idle

	defer func() {
		s.lastCPUTotal, s.lastCPUBusy = total, busy
	}()
	if s.lastCPUTotal == 0 {
		return 0
	}
	dt := total - s.lastCPUTotal
	if dt <= 0 {
		return 0
	}
	pct := (busy - s.lastCPUBusy) / dt * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return round2(pct)
}

func (s *Sampler) ramPct() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	var total, available float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "MemTotal:":
			total, _ = strconv.ParseFloat(parts[1], 64)
		case "MemAvailable:":
			available, _ = strconv.ParseFloat(parts[1], 64)
		}
	}
	if total == 0 {
		return 0
	}
	return round2((total - available) / total * 100)
}

func (s *Sampler) netMbps() (in, out float64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var rx, tx uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name != s.iface {
			continue
		}
		// Fields: bytes packets errs drop fifo frame compressed multicast
		//         (rx)  ...                                                (tx)bytes packets ...
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 9 {
			break
		}
		rx, _ = strconv.ParseUint(fields[0], 10, 64)
		tx, _ = strconv.ParseUint(fields[8], 10, 64)
		break
	}

	now := time.Now()
	defer func() {
		s.lastNetRx, s.lastNetTx, s.lastNetAt = rx, tx, now
	}()
	if s.lastNetAt.IsZero() || rx < s.lastNetRx || tx < s.lastNetTx {
		return 0, 0
	}
	dt := now.Sub(s.lastNetAt).Seconds()
	if dt <= 0 {
		return 0, 0
	}
	const bitsPerByte = 8.0
	in = round2(float64(rx-s.lastNetRx) * bitsPerByte / dt / 1e6)
	out = round2(float64(tx-s.lastNetTx) * bitsPerByte / dt / 1e6)
	return
}

func (s *Sampler) uptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(b))
	if len(parts) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(parts[0], 64)
	return int64(secs)
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// Version is reported in heartbeat so the panel can flag agents that need
// upgrading. Bump on agent behavior changes, not on every commit.
const Version = "0.1.0"
