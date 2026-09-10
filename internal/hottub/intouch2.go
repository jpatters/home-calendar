// Package hottub reads water temperature, setpoint, and heater state from a
// Gecko in.touch2 hot tub WiFi module over the LAN.
//
// The module speaks a small UDP protocol on port 10022 (the same one the
// in.touch2 phone app and the geckolib Home Assistant integration use):
//
//   - "<HELLO>1</HELLO>" is answered with "<HELLO>{spaID}|{name}</HELLO>".
//   - Everything else is wrapped as
//     "<PACKT><SRCCN>{client}</SRCCN><DESCN>{spaID}</DESCN><DATAS>{payload}</DATAS></PACKT>".
//   - "SFILE" returns "FILES,{config}.xml,{log}.xml" naming the spa pack, which
//     fixes the byte offsets of every value in the status block.
//   - "STATU" + seq + start + length (big-endian) reads a slice of the status
//     block and is answered with "STATV" + seq + next + length + bytes.
//
// Only the inYT pack with log structure v66 is supported; the offsets below
// come from geckolib's inyt-log-66 definition and were verified against a real
// module. Any other pack is rejected rather than decoded with the wrong offsets.
package hottub

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

const (
	DefaultPort = "10022"

	clientID         = "IOShome-calendar"
	supportedLogFile = "inYT_S66"
	requestTimeout   = 2 * time.Second
	requestAttempts  = 2

	// Offsets into the 1024-byte status block for inYT log v66.
	blockStart     = 260
	blockLen       = 20
	heatingPos     = 260 // bits 5-6: non-zero while the heater is on
	setpointPos    = 275 // uint16 RealSetPointG
	waterTempPos   = 277 // uint16 DisplayedTempG
	tempInvalidPos = 279 // bit 2: TempNotValid
)

type Fetcher struct {
	mu       sync.RWMutex
	snapshot *types.HotTubSnapshot

	cancel   context.CancelFunc
	doneWG   sync.WaitGroup
	onUpdate func(*types.HotTubSnapshot)
}

func New(onUpdate func(*types.HotTubSnapshot)) *Fetcher {
	return &Fetcher{onUpdate: onUpdate}
}

func (f *Fetcher) Snapshot() *types.HotTubSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.snapshot == nil {
		return nil
	}
	s := *f.snapshot
	return &s
}

func (f *Fetcher) Start(parent context.Context, h types.HotTub, interval time.Duration) {
	f.Stop()
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.doneWG.Add(1)
	go f.loop(ctx, h, interval)
}

func (f *Fetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
		f.doneWG.Wait()
		f.cancel = nil
	}
	f.mu.Lock()
	f.snapshot = nil
	f.mu.Unlock()
}

func (f *Fetcher) RefreshNow(ctx context.Context, h types.HotTub) {
	f.fetch(ctx, h)
}

func (f *Fetcher) loop(ctx context.Context, h types.HotTub, interval time.Duration) {
	defer f.doneWG.Done()
	f.fetch(ctx, h)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetch(ctx, h)
		}
	}
}

func (f *Fetcher) fetch(ctx context.Context, h types.HotTub) {
	if strings.TrimSpace(h.Host) == "" {
		f.mu.Lock()
		hadSnapshot := f.snapshot != nil
		f.snapshot = nil
		f.mu.Unlock()
		if hadSnapshot && f.onUpdate != nil {
			f.onUpdate(nil)
		}
		return
	}
	snap, err := Read(ctx, h.Host)
	if err != nil {
		// UDP drops and the module's RF link to the spa both come and go;
		// keep the last good snapshot and try again on the next tick.
		log.Printf("hottub: %v", err)
		return
	}
	f.mu.Lock()
	f.snapshot = snap
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
}

// Read queries the in.touch2 module at host ("ip" or "ip:port") and returns
// the current water temperature, setpoint, and heater state in Fahrenheit.
func Read(ctx context.Context, host string) (*types.HotTubSnapshot, error) {
	addr := withDefaultPort(host)
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("hottub: dial %q: %w", addr, err)
	}
	defer conn.Close()
	// A cancelled context must interrupt a blocking Read immediately so
	// Stop() (and therefore config saves) never waits out a request timeout.
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Now()) })
	defer stop()
	c := &client{conn: conn}

	hello, err := c.exchange(ctx, []byte("<HELLO>1</HELLO>"), []byte("<HELLO>"))
	if err != nil {
		return nil, fmt.Errorf("hottub: discover: %w", err)
	}
	spaID, _, _ := bytes.Cut(bytes.TrimSuffix(bytes.TrimPrefix(hello, []byte("<HELLO>")), []byte("</HELLO>")), []byte("|"))
	if len(spaID) == 0 {
		return nil, fmt.Errorf("hottub: discover: bad reply %q", hello)
	}
	c.spaID = spaID

	files, err := c.request(ctx, []byte("SFILE"), []byte("FILES"))
	if err != nil {
		return nil, fmt.Errorf("hottub: read spa pack: %w", err)
	}
	if err := checkSpaPack(files); err != nil {
		return nil, err
	}

	req := append([]byte("STATU"), 1)
	req = binary.BigEndian.AppendUint16(req, blockStart)
	req = binary.BigEndian.AppendUint16(req, blockLen)
	statv, err := c.request(ctx, req, []byte("STATV"))
	if err != nil {
		return nil, fmt.Errorf("hottub: read status: %w", err)
	}
	if len(statv) < 8 || int(statv[7]) != blockLen || len(statv) < 8+blockLen {
		return nil, fmt.Errorf("hottub: read status: short reply %q", statv)
	}
	return decode(statv[8 : 8+blockLen])
}

// withDefaultPort appends the in.touch2 port unless host already carries one.
// Bare IPv6 literals fail SplitHostPort too and are bracketed by JoinHostPort.
func withDefaultPort(host string) string {
	host = strings.TrimSpace(host)
	if _, _, err := net.SplitHostPort(host); err != nil {
		return net.JoinHostPort(host, DefaultPort)
	}
	return host
}

func checkSpaPack(files []byte) error {
	// e.g. "FILES,inYT_C65.xml,inYT_S66.xml"
	parts := strings.Split(string(files), ",")
	if len(parts) != 3 {
		return fmt.Errorf("hottub: unexpected spa pack reply %q", files)
	}
	logFile := strings.TrimSuffix(parts[2], ".xml")
	if logFile != supportedLogFile {
		return fmt.Errorf("hottub: unsupported spa pack %q (only %s is supported)", files, supportedLogFile)
	}
	return nil
}

func decode(block []byte) (*types.HotTubSnapshot, error) {
	if block[tempInvalidPos-blockStart]&0x04 != 0 {
		return nil, errors.New("hottub: module reports water temperature not valid")
	}
	// Temperatures are tenths of a degree Fahrenheit above freezing.
	toF := func(raw uint16) float64 { return float64(raw)/10 + 32 }
	return &types.HotTubSnapshot{
		UpdatedAt:    time.Now(),
		TemperatureF: toF(binary.BigEndian.Uint16(block[waterTempPos-blockStart:])),
		TargetF:      toF(binary.BigEndian.Uint16(block[setpointPos-blockStart:])),
		Heating:      (block[heatingPos-blockStart]>>5)&0x03 != 0,
	}, nil
}

type client struct {
	conn  net.Conn
	spaID []byte
}

// request wraps payload in a PACKT envelope and returns the DATAS content of
// the first reply whose content starts with want.
func (c *client) request(ctx context.Context, payload, want []byte) ([]byte, error) {
	var frame []byte
	frame = append(frame, "<PACKT><SRCCN>"+clientID+"</SRCCN><DESCN>"...)
	frame = append(frame, c.spaID...)
	frame = append(frame, "</DESCN><DATAS>"...)
	frame = append(frame, payload...)
	frame = append(frame, "</DATAS></PACKT>"...)
	reply, err := c.exchange(ctx, frame, append([]byte("<DATAS>"), want...))
	if err != nil {
		return nil, err
	}
	i := bytes.Index(reply, []byte("<DATAS>"))
	j := bytes.LastIndex(reply, []byte("</DATAS>"))
	if i < 0 || j < i {
		return nil, fmt.Errorf("malformed reply %q", reply)
	}
	return reply[i+len("<DATAS>") : j], nil
}

// exchange sends msg and waits for a datagram containing want, retrying once
// on timeout since UDP offers no delivery guarantee. The module's replies do
// not close their tags consistently, so matching is done on content only.
func (c *client) exchange(ctx context.Context, msg, want []byte) ([]byte, error) {
	buf := make([]byte, 4096)
	for attempt := 0; attempt < requestAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(requestTimeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := c.conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
		if _, err := c.conn.Write(msg); err != nil {
			return nil, err
		}
		for {
			n, err := c.conn.Read(buf)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					if ctxErr := ctx.Err(); ctxErr != nil {
						return nil, ctxErr
					}
					break
				}
				return nil, err
			}
			if bytes.Contains(buf[:n], want) {
				return append([]byte(nil), buf[:n]...), nil
			}
		}
	}
	return nil, fmt.Errorf("no reply from module after %d attempts", requestAttempts)
}
