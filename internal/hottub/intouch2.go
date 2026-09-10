// Package hottub reads water temperature, setpoint, and heater state from a
// Gecko in.touch2 hot tub WiFi module over the LAN, and can change the
// setpoint.
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
//     block and is answered with "STATV" + seq + next + length + bytes. The
//     module truncates replies longer than about 39 bytes, so reads are made
//     in 20-byte chunks.
//   - "SPACK" + seq + packType + length + 70 + configVersion + logVersion +
//     position + value writes a value into the status block and is
//     acknowledged with "PACKS".
//
// Only the inYT pack with log structure v66 is supported; the offsets below
// come from geckolib's inyt-cfg-65 and inyt-log-66 definitions and were
// verified against a real module. Any other pack is rejected rather than
// decoded with the wrong offsets.
package hottub

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jpatters/home-calendar/internal/types"
)

// ErrInvalidTarget reports a requested setpoint the tub cannot be set to.
var ErrInvalidTarget = errors.New("hottub: invalid target temperature")

const (
	DefaultPort = "10022"

	clientID            = "IOShome-calendar"
	supportedConfigFile = "inYT_C65"
	supportedLogFile    = "inYT_S66"
	configVersion       = 65
	logVersion          = 66
	requestTimeout      = 2 * time.Second
	requestAttempts     = 2

	packTypeInYT        = 10
	packCommandSetValue = 70
	// Commands carry a sequence byte from 192..255, as the official app does;
	// a session sends at most one, so it never needs to advance.
	commandSeq = 192

	// Offsets into the 1024-byte status block. The config struct (inYT config
	// v65) occupies bytes 0..255 and the log struct (inYT log v66) follows.
	// Each value lives in one of three statusChunk-byte reads.
	statusChunk   = 20
	setpointChunk = 0
	limitsChunk   = 60
	stateChunk    = 260

	setpointPos    = 1   // uint16 SetpointG, the user's target
	minSetpointPos = 66  // uint16 MinSetpointG
	maxSetpointPos = 68  // uint16 MaxSetpointG
	heatingPos     = 260 // bits 5-6: non-zero while the heater is on
	waterTempPos   = 277 // uint16 DisplayedTempG
	tempInvalidPos = 279 // bit 2: TempNotValid
)

type Fetcher struct {
	mu       sync.RWMutex
	snapshot *types.HotTubSnapshot

	// ioMu serialises polls and writes so a poll that started before a write
	// cannot publish the old setpoint after it.
	ioMu     sync.Mutex
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

// SetTarget changes the tub's setpoint and publishes the reading taken
// afterwards.
func (f *Fetcher) SetTarget(ctx context.Context, h types.HotTub, targetF float64) (*types.HotTubSnapshot, error) {
	f.ioMu.Lock()
	defer f.ioMu.Unlock()
	snap, err := SetTarget(ctx, h.Host, targetF)
	if err != nil {
		return nil, err
	}
	f.publish(snap)
	return snap, nil
}

func (f *Fetcher) publish(snap *types.HotTubSnapshot) {
	f.mu.Lock()
	f.snapshot = snap
	f.mu.Unlock()
	if f.onUpdate != nil {
		f.onUpdate(snap)
	}
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
	f.ioMu.Lock()
	defer f.ioMu.Unlock()
	snap, err := Read(ctx, h.Host)
	if err != nil {
		// UDP drops and the module's RF link to the spa both come and go;
		// keep the last good snapshot and try again on the next tick.
		log.Printf("hottub: %v", err)
		return
	}
	f.publish(snap)
}

// Read queries the in.touch2 module at host ("ip" or "ip:port") and returns
// the current water temperature, setpoint, setpoint limits, and heater state
// in Fahrenheit.
func Read(ctx context.Context, host string) (*types.HotTubSnapshot, error) {
	s, err := dial(ctx, host)
	if err != nil {
		return nil, err
	}
	defer s.close()
	return s.readSnapshot(ctx)
}

// SetTarget sets the tub's setpoint to targetF, a whole number of degrees
// within the limits the module reports, and returns the reading taken after
// the module acknowledged the change. Out-of-range or fractional targets are
// rejected with ErrInvalidTarget without contacting the module.
func SetTarget(ctx context.Context, host string, targetF float64) (*types.HotTubSnapshot, error) {
	if targetF != math.Trunc(targetF) {
		return nil, fmt.Errorf("%w: %v is not a whole degree", ErrInvalidTarget, targetF)
	}
	s, err := dial(ctx, host)
	if err != nil {
		return nil, err
	}
	defer s.close()
	minF, maxF, err := s.readLimits(ctx)
	if err != nil {
		return nil, err
	}
	if targetF < minF || targetF > maxF {
		return nil, fmt.Errorf("%w: %v°F is outside %v–%v°F", ErrInvalidTarget, targetF, minF, maxF)
	}
	if err := s.writeWord(ctx, setpointPos, fromF(targetF)); err != nil {
		return nil, fmt.Errorf("hottub: set target: %w", err)
	}
	return s.readSnapshot(ctx)
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

// checkSpaPack verifies the FILES reply names the one spa pack whose offsets
// this package knows.
func checkSpaPack(files []byte) error {
	// e.g. "FILES,inYT_C65.xml,inYT_S66.xml"
	parts := strings.Split(string(files), ",")
	if len(parts) != 3 {
		return fmt.Errorf("hottub: unexpected spa pack reply %q", files)
	}
	configFile := strings.TrimSuffix(parts[1], ".xml")
	logFile := strings.TrimSuffix(parts[2], ".xml")
	if configFile != supportedConfigFile || logFile != supportedLogFile {
		return fmt.Errorf("hottub: unsupported spa pack %q (only %s/%s is supported)", files, supportedConfigFile, supportedLogFile)
	}
	return nil
}

// Temperatures are tenths of a degree Fahrenheit above freezing.
func toF(raw uint16) float64 { return float64(raw)/10 + 32 }
func fromF(f float64) uint16 { return uint16((f - 32) * 10) }

type session struct {
	conn      net.Conn
	stop      func() bool
	spaID     []byte
	statusSeq byte
}

// dial connects to the module, discovers its identifier, and verifies the spa
// pack so every later offset is known to be valid.
func dial(ctx context.Context, host string) (*session, error) {
	addr := withDefaultPort(host)
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("hottub: dial %q: %w", addr, err)
	}
	s := &session{conn: conn}
	// A cancelled context must interrupt a blocking Read immediately so
	// Stop() (and therefore config saves) never waits out a request timeout.
	s.stop = context.AfterFunc(ctx, func() { conn.SetDeadline(time.Now()) })

	hello, err := s.exchange(ctx, []byte("<HELLO>1</HELLO>"), []byte("<HELLO>"))
	if err != nil {
		s.close()
		return nil, fmt.Errorf("hottub: discover: %w", err)
	}
	spaID, _, _ := bytes.Cut(bytes.TrimSuffix(bytes.TrimPrefix(hello, []byte("<HELLO>")), []byte("</HELLO>")), []byte("|"))
	if len(spaID) == 0 {
		s.close()
		return nil, fmt.Errorf("hottub: discover: bad reply %q", hello)
	}
	s.spaID = spaID

	files, err := s.request(ctx, []byte("SFILE"), []byte("FILES"))
	if err != nil {
		s.close()
		return nil, fmt.Errorf("hottub: read spa pack: %w", err)
	}
	if err := checkSpaPack(files); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *session) close() {
	s.stop()
	s.conn.Close()
}

func (s *session) readSnapshot(ctx context.Context) (*types.HotTubSnapshot, error) {
	setpoint, err := s.readStatus(ctx, setpointChunk)
	if err != nil {
		return nil, err
	}
	minF, maxF, err := s.readLimits(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.readStatus(ctx, stateChunk)
	if err != nil {
		return nil, err
	}
	if state[tempInvalidPos-stateChunk]&0x04 != 0 {
		return nil, errors.New("hottub: module reports water temperature not valid")
	}
	return &types.HotTubSnapshot{
		UpdatedAt:    time.Now(),
		TemperatureF: toF(binary.BigEndian.Uint16(state[waterTempPos-stateChunk:])),
		TargetF:      toF(binary.BigEndian.Uint16(setpoint[setpointPos-setpointChunk:])),
		MinTargetF:   minF,
		MaxTargetF:   maxF,
		Heating:      (state[heatingPos-stateChunk]>>5)&0x03 != 0,
	}, nil
}

// readLimits returns the lowest and highest setpoint the spa pack allows.
func (s *session) readLimits(ctx context.Context) (minF, maxF float64, err error) {
	limits, err := s.readStatus(ctx, limitsChunk)
	if err != nil {
		return 0, 0, err
	}
	return toF(binary.BigEndian.Uint16(limits[minSetpointPos-limitsChunk:])),
		toF(binary.BigEndian.Uint16(limits[maxSetpointPos-limitsChunk:])), nil
}

// readStatus returns statusChunk bytes of the status block from start. Each
// request carries its own sequence byte so a late reply to an earlier read
// (after a retry) is never mistaken for this one.
func (s *session) readStatus(ctx context.Context, start int) ([]byte, error) {
	s.statusSeq++
	req := append([]byte("STATU"), s.statusSeq)
	req = binary.BigEndian.AppendUint16(req, uint16(start))
	req = binary.BigEndian.AppendUint16(req, statusChunk)
	statv, err := s.request(ctx, req, []byte{'S', 'T', 'A', 'T', 'V', s.statusSeq})
	if err != nil {
		return nil, fmt.Errorf("hottub: read status: %w", err)
	}
	if len(statv) < 8 || int(statv[7]) != statusChunk || len(statv) < 8+statusChunk {
		return nil, fmt.Errorf("hottub: read status: short reply %q", statv)
	}
	return statv[8 : 8+statusChunk], nil
}

// writeWord sets the 16-bit value at pos in the status block and waits for
// the module's acknowledgement.
func (s *session) writeWord(ctx context.Context, pos int, value uint16) error {
	req := append([]byte("SPACK"), commandSeq, packTypeInYT, 5+2, packCommandSetValue, configVersion, logVersion)
	req = binary.BigEndian.AppendUint16(req, uint16(pos))
	req = binary.BigEndian.AppendUint16(req, value)
	_, err := s.request(ctx, req, []byte("PACKS"))
	return err
}

// request wraps payload in a PACKT envelope and returns the DATAS content of
// the first reply whose content starts with want.
func (s *session) request(ctx context.Context, payload, want []byte) ([]byte, error) {
	var frame []byte
	frame = append(frame, "<PACKT><SRCCN>"+clientID+"</SRCCN><DESCN>"...)
	frame = append(frame, s.spaID...)
	frame = append(frame, "</DESCN><DATAS>"...)
	frame = append(frame, payload...)
	frame = append(frame, "</DATAS></PACKT>"...)
	reply, err := s.exchange(ctx, frame, append([]byte("<DATAS>"), want...))
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
func (s *session) exchange(ctx context.Context, msg, want []byte) ([]byte, error) {
	buf := make([]byte, 4096)
	for attempt := 0; attempt < requestAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(requestTimeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := s.conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
		if _, err := s.conn.Write(msg); err != nil {
			return nil, err
		}
		for {
			n, err := s.conn.Read(buf)
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
