package auth

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Bans is the multi-day IP ban list. After MaxFailures failed logins (or wrong
// device keys) within a day, a public IP is banned for BanDays and every request
// from it gets 403. The list lives in <state dir>/bans.yml, so a restart does
// not lift a ban; only `natlas unban <ip>` (or editing the file) does.
//
// Private, loopback and link-local addresses are never banned, only locked out
// for 30 minutes: home devices are trusted, and a wrong proxy header must never
// ban the proxy itself (that would lock everyone out for days).
type Bans struct {
	path string // "" = memory only (tests, phone app)
	days int
	max  int

	mu   sync.Mutex
	mod  time.Time // file version last read
	size int64
	f    banFile
}

type banFile struct {
	Bans     map[string]Ban         `yaml:"bans"`
	Failures map[string][]time.Time `yaml:"failures,omitempty"`
}

// Ban is one banned address.
type Ban struct {
	Since  time.Time `yaml:"since"`
	Until  time.Time `yaml:"until"`
	Reason string    `yaml:"reason"`
}

const failWindow = 24 * time.Hour // failures older than this are forgotten

// OpenBans loads (or starts) the ban list at path.
func OpenBans(path string, days, max int) *Bans {
	b := &Bans{path: path, days: days, max: max}
	b.f = banFile{Bans: map[string]Ban{}, Failures: map[string][]time.Time{}}
	b.mu.Lock()
	b.reload()
	b.mu.Unlock()
	return b
}

// Bannable is false for home, loopback and link-local addresses.
func Bannable(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && !(p.IsPrivate() || p.IsLoopback() || p.IsLinkLocalUnicast() || p.IsUnspecified())
}

// reload re-reads the file when it changed (an unban from the CLI). Caller holds mu.
func (b *Bans) reload() {
	if b.path == "" {
		return
	}
	st, err := os.Stat(b.path)
	if err != nil || (st.ModTime().Equal(b.mod) && st.Size() == b.size) {
		return
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		return
	}
	var f banFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return // a broken file keeps the bans already in memory
	}
	if f.Bans == nil {
		f.Bans = map[string]Ban{}
	}
	if f.Failures == nil {
		f.Failures = map[string][]time.Time{}
	}
	b.f, b.mod, b.size = f, st.ModTime(), st.Size()
}

// save writes the file atomically. Caller holds mu.
func (b *Bans) save() error {
	if b.path == "" {
		return nil
	}
	now := time.Now()
	for ip, ban := range b.f.Bans { // expired bans and old failures go
		if now.After(ban.Until) {
			delete(b.f.Bans, ip)
		}
	}
	for ip, list := range b.f.Failures {
		if kept := recentTimes(list, now); len(kept) > 0 {
			b.f.Failures[ip] = kept
		} else {
			delete(b.f.Failures, ip)
		}
	}
	data, err := yaml.Marshal(b.f)
	if err != nil {
		return err
	}
	data = append([]byte("# Natlas IP bans. Lift one with: natlas unban <ip>\n"), data...)
	if err := os.MkdirAll(filepath.Dir(b.path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(b.path), ".bans-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), b.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if st, err := os.Stat(b.path); err == nil {
		b.mod, b.size = st.ModTime(), st.Size()
	}
	return nil
}

func recentTimes(list []time.Time, now time.Time) []time.Time {
	var kept []time.Time
	for _, t := range list {
		if now.Sub(t) < failWindow {
			kept = append(kept, t)
		}
	}
	return kept
}

// Banned reports whether ip is banned, and until when.
func (b *Bans) Banned(ip string) (time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reload()
	ban, ok := b.f.Bans[ip]
	if !ok || time.Now().After(ban.Until) {
		return time.Time{}, false
	}
	return ban.Until, true
}

// Fail records a failure; returns the attempts left and whether ip is now banned.
func (b *Bans) Fail(ip, reason string) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reload()
	now := time.Now()
	list := append(recentTimes(b.f.Failures[ip], now), now)
	b.f.Failures[ip] = list
	left := b.max - len(list)
	banned := left <= 0
	if banned {
		b.f.Bans[ip] = Ban{Since: now, Until: now.Add(time.Duration(b.days) * 24 * time.Hour), Reason: reason}
		delete(b.f.Failures, ip)
	}
	if err := b.save(); err != nil {
		fmt.Fprintf(os.Stderr, "[auth] cannot save %s: %v\n", b.path, err)
	}
	return left, banned
}

// Clear forgets the failures of ip (after a good login).
func (b *Bans) Clear(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reload()
	if _, ok := b.f.Failures[ip]; ok {
		delete(b.f.Failures, ip)
		b.save()
	}
}

// List returns the current bans, soonest to end first.
func (b *Bans) List() []struct {
	IP string
	Ban
} {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reload()
	var out []struct {
		IP string
		Ban
	}
	for ip, ban := range b.f.Bans {
		if time.Now().Before(ban.Until) {
			out = append(out, struct {
				IP string
				Ban
			}{ip, ban})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Until.Before(out[j].Until) })
	return out
}

// Unban lifts the ban on ip ("all" lifts every ban); reports how many were lifted.
func (b *Bans) Unban(ip string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reload()
	n := 0
	for k := range b.f.Bans {
		if ip == "all" || k == ip {
			delete(b.f.Bans, k)
			delete(b.f.Failures, k)
			n++
		}
	}
	return n, b.save()
}
