package ical

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/jpatters/home-calendar/internal/types"
)

type Fetcher struct {
	client    *http.Client
	mu        sync.RWMutex
	events    []types.Event
	lastFetch time.Time
	lastErr   error

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func([]types.Event)
}

func New(onUpdate func([]types.Event)) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: 30 * time.Second},
		onUpdate: onUpdate,
	}
}

func (f *Fetcher) Events() []types.Event {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]types.Event, len(f.events))
	copy(out, f.events)
	return out
}

func (f *Fetcher) Status() (time.Time, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastFetch, f.lastErr
}

// Start launches the background fetch loop. Calling Start again cancels the previous loop.
func (f *Fetcher) Start(parent context.Context, cals []types.Calendar, interval time.Duration) {
	f.Stop()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, cals, interval)
}

func (f *Fetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
		f.doneWG.Wait()
		f.cancel = nil
	}
	f.mu.Lock()
	f.events = nil
	f.lastErr = nil
	f.mu.Unlock()
}

// RefreshNow triggers an immediate fetch with the given calendars.
func (f *Fetcher) RefreshNow(ctx context.Context, cals []types.Calendar) {
	f.fetchAll(ctx, cals)
}

func (f *Fetcher) loop(ctx context.Context, cals []types.Calendar, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetchAll(ctx, cals)
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.fetchAll(ctx, cals)
		}
	}
}

func (f *Fetcher) fetchAll(ctx context.Context, cals []types.Calendar) {
	var all []types.Event
	var firstErr error
	for _, c := range cals {
		evs, err := f.fetchOne(ctx, c)
		if err != nil {
			log.Printf("ical: fetch %q (%s): %v", c.Name, c.ID, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, evs...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start.Before(all[j].Start) })

	f.mu.Lock()
	f.events = all
	f.lastFetch = time.Now()
	f.lastErr = firstErr
	f.mu.Unlock()

	if f.onUpdate != nil {
		f.onUpdate(all)
	}
}

func (f *Fetcher) fetchOne(ctx context.Context, c types.Calendar) ([]types.Event, error) {
	if strings.TrimSpace(c.URL) == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "home-calendar/1.0")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	cal, err := ics.ParseCalendar(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	windowStart := now.AddDate(0, -3, 0)
	windowEnd := now.AddDate(1, 0, 0)
	return expandCalendar(cal, c, windowStart, windowEnd), nil
}

func expandCalendar(cal *ics.Calendar, c types.Calendar, windowStart, windowEnd time.Time) []types.Event {
	var out []types.Event
	for _, ev := range cal.Events() {
		start, err := ev.GetStartAt()
		if err != nil {
			continue
		}
		end, err := ev.GetEndAt()
		if err != nil {
			end = start.Add(time.Hour)
		}
		title := stringProp(ev, ics.ComponentPropertySummary)
		location := stringProp(ev, ics.ComponentPropertyLocation)
		description := stringProp(ev, ics.ComponentPropertyDescription)
		uid := stringProp(ev, ics.ComponentPropertyUniqueId)
		allDay := isAllDay(ev)

		// Expand RRULE instances by delegating to the library's helper.
		instances := expandInstances(ev, start, end, windowStart, windowEnd)
		for i, inst := range instances {
			if inst.end.Before(windowStart) || inst.start.After(windowEnd) {
				continue
			}
			id := fmt.Sprintf("%s::%s::%d", c.ID, uid, i)
			out = append(out, types.Event{
				ID:            id,
				CalendarID:    c.ID,
				CalendarName:  c.Name,
				CalendarColor: c.Color,
				Title:         title,
				Start:         inst.start,
				End:           inst.end,
				AllDay:        allDay,
				Location:      location,
				Description:   description,
			})
		}
	}
	return out
}

type instance struct {
	start, end time.Time
}

func expandInstances(ev *ics.VEvent, start, end, windowStart, windowEnd time.Time) []instance {
	rrule := stringProp(ev, ics.ComponentPropertyRrule)
	if rrule == "" {
		return []instance{{start, end}}
	}
	// Best-effort RRULE handling for common cases (FREQ=DAILY/WEEKLY/MONTHLY/YEARLY).
	// Falls back to the single occurrence if parsing fails.
	rule := parseSimpleRRule(rrule)
	if rule == nil {
		return []instance{{start, end}}
	}
	duration := end.Sub(start)
	var out []instance
	cur := start
	count := 0
	maxIter := 1000
	for i := 0; i < maxIter; i++ {
		if rule.until != nil && cur.After(*rule.until) {
			break
		}
		if rule.count > 0 && count >= rule.count {
			break
		}
		if cur.After(windowEnd) {
			break
		}
		if !cur.Before(windowStart.AddDate(0, 0, -1)) {
			out = append(out, instance{cur, cur.Add(duration)})
		}
		cur = rule.advance(cur)
		count++
	}
	if len(out) == 0 {
		return []instance{{start, end}}
	}
	return out
}

type simpleRule struct {
	freq     string
	interval int
	count    int
	until    *time.Time
}

func (r *simpleRule) advance(t time.Time) time.Time {
	n := r.interval
	if n <= 0 {
		n = 1
	}
	switch r.freq {
	case "DAILY":
		return t.AddDate(0, 0, n)
	case "WEEKLY":
		return t.AddDate(0, 0, 7*n)
	case "MONTHLY":
		return t.AddDate(0, n, 0)
	case "YEARLY":
		return t.AddDate(n, 0, 0)
	}
	return t.AddDate(100, 0, 0)
}

func parseSimpleRRule(s string) *simpleRule {
	r := &simpleRule{interval: 1}
	parts := strings.Split(s, ";")
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.ToUpper(kv[0]) {
		case "FREQ":
			r.freq = strings.ToUpper(kv[1])
		case "INTERVAL":
			var n int
			fmt.Sscanf(kv[1], "%d", &n)
			if n > 0 {
				r.interval = n
			}
		case "COUNT":
			fmt.Sscanf(kv[1], "%d", &r.count)
		case "UNTIL":
			if t, err := parseRRuleTime(kv[1]); err == nil {
				r.until = &t
			}
		}
	}
	if r.freq == "" {
		return nil
	}
	return r
}

func parseRRuleTime(s string) (time.Time, error) {
	layouts := []string{"20060102T150405Z", "20060102T150405", "20060102"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable RRULE time %q", s)
}

func stringProp(ev *ics.VEvent, name ics.ComponentProperty) string {
	p := ev.GetProperty(name)
	if p == nil {
		return ""
	}
	return p.Value
}

func isAllDay(ev *ics.VEvent) bool {
	p := ev.GetProperty(ics.ComponentPropertyDtStart)
	if p == nil {
		return false
	}
	for k, v := range p.ICalParameters {
		if strings.EqualFold(k, "VALUE") {
			for _, x := range v {
				if strings.EqualFold(x, "DATE") {
					return true
				}
			}
		}
	}
	return false
}
