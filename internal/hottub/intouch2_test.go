package hottub_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jpatters/home-calendar/internal/hottub"
	"github.com/jpatters/home-calendar/internal/types"
)

// realStatus is the inYT log struct (bytes 256..479) read verbatim from an
// in.touch2 module (spa pack inYT_C65/inYT_S66) on 2026-09-10 while the tub was
// heating from 67°F toward a 102°F setpoint. The field offsets the tests rely
// on (heating bits 5-6 of byte 260, setpoint word at 275, displayed temperature
// word at 277, TempNotValid bit 2 of byte 279) match geckolib's inyt-log-66
// definition.
func realStatus(t *testing.T) []byte {
	t.Helper()
	hexParts := []string{
		"1000000024000000000000000000ffffff000002bc015e02010040000000400e",
		"000a4b003d3700414201630500001d0000000000000000038400000000015300",
		"0000000000000000000000000000000000000000000000000000000000000000",
		"00000001c10000000000000000ff4b00000000ffde0601000008000000000000",
		"0000000000007fff000000000000000000000000000000000000000000000000",
		"0000000000000000000000000000000000000000000000000000000000000000",
		"0102030405060708090a0b0c0d0e0f101112131415333435369798999affffff",
	}
	var out []byte
	for _, h := range hexParts {
		b, err := hex.DecodeString(h)
		if err != nil {
			t.Fatalf("bad hex fixture: %v", err)
		}
		out = append(out, b...)
	}
	return out
}

// realConfig is the first 80 bytes of the inYT config struct (bytes 0..255)
// read from the same module on 2026-09-10: SetpointG word at 1 (700 = 102°F),
// TempUnits byte at 33 (0 = °F), MinSetpointG word at 66 (270 = 59°F),
// MaxSetpointG word at 68 (740 = 106°F), matching geckolib's inyt-cfg-65.
func realConfig(t *testing.T) []byte {
	t.Helper()
	b, err := hex.DecodeString("0202bc02131308001800090000010c0b020000000000000000000e00000000000100011e000c00010400000000000000000011000128147878021402040828280073010e02e40001200100000100000f")
	if err != nil {
		t.Fatalf("bad hex fixture: %v", err)
	}
	return b
}

// testCtx bounds every Read so a framing bug the fake silently ignores fails
// the test quickly instead of hanging until the go test timeout.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

type fakeModule struct {
	files  string
	status []byte // log struct starting at offset 256
	config []byte // config struct starting at offset 0; realConfig when nil
	silent bool
	// ignoreWrites drops SPACK commands without acknowledging them.
	ignoreWrites bool
	// dropFirst ignores the first STATU request the fake sees so only a
	// resend gets an answer.
	dropFirst bool
	// noisy sends an unsolicited STATP push (which a real module emits to
	// connected clients) before every reply.
	noisy bool
	// slowFirst answers the first STATU only once its resend arrives, and
	// then answers both, as a module that was merely slow does.
	slowFirst bool

	mu     sync.Mutex
	writes [][]byte // DATAS payload of every SPACK received
}

// Writes returns the SPACK payloads the fake has received so far.
func (m *fakeModule) Writes() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]byte(nil), m.writes...)
}

// maxStatusChunk mirrors the real module, which answers a STATU request for
// more than ~39 bytes with a truncated chunk.
const maxStatusChunk = 39

// serve runs a fake in.touch2 module on loopback and returns its address. It
// mimics the real module's quirks: only the discovery hello ("1") is answered,
// and the PACKT reply has a mismatched <DESCN>…</SRCCN> tag pair.
func serve(t *testing.T, m *fakeModule) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	block := make([]byte, 1024)
	if m.config == nil {
		copy(block, realConfig(t))
	} else {
		copy(block, m.config)
	}
	copy(block[256:], m.status)
	go func() {
		buf := make([]byte, 4096)
		dropped := false
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if m.silent {
				continue
			}
			msg := buf[:n]
			if (m.dropFirst || m.slowFirst) && !dropped && bytes.Contains(msg, []byte("STATU")) {
				dropped = true
				continue
			}
			if m.noisy {
				pc.WriteTo([]byte("<PACKT><SRCCN>SPAfc:0f:e7:d9:c5:5b</SRCCN><DESCN>client</DESCN><DATAS>STATP\x01\x01\x0b\x00\x14</DATAS></PACKT>"), addr)
			}
			if bytes.Equal(msg, []byte("<HELLO>1</HELLO>")) {
				pc.WriteTo([]byte("<HELLO>SPAfc:0f:e7:d9:c5:5b|My Spa</HELLO>"), addr)
				continue
			}
			i := bytes.Index(msg, []byte("<DATAS>"))
			j := bytes.LastIndex(msg, []byte("</DATAS>"))
			if i < 0 || j < 0 {
				continue
			}
			data := msg[i+7 : j]
			var reply []byte
			switch {
			case bytes.Equal(data, []byte("SFILE")):
				reply = []byte(m.files)
			case bytes.HasPrefix(data, []byte("STATU")) && len(data) == 10:
				start := int(binary.BigEndian.Uint16(data[6:8]))
				length := int(binary.BigEndian.Uint16(data[8:10]))
				if start+length > len(block) {
					continue
				}
				if length > maxStatusChunk {
					length = maxStatusChunk
				}
				// The real module always answers with sequence byte 0.
				reply = append([]byte("STATV"), 0, 0, byte(length))
				reply = append(reply, block[start:start+length]...)
				if m.slowFirst && start == 0 {
					frame := append([]byte("<PACKT><SRCCN>SPAfc:0f:e7:d9:c5:5b</SRCCN><DESCN>client</SRCCN><DESCN>SPAfc:0f:e7:</DESCN><DATAS>"), reply...)
					pc.WriteTo(append(frame, []byte("</DATAS></PACKT>")...), addr)
				}
			case bytes.HasPrefix(data, []byte("SPACK")) && len(data) >= 13:
				m.mu.Lock()
				m.writes = append(m.writes, append([]byte(nil), data...))
				m.mu.Unlock()
				if m.ignoreWrites {
					continue
				}
				// SPACK seq type len cmd cfgver logver pos(2) value(len-5)
				pos := int(binary.BigEndian.Uint16(data[11:13]))
				value := data[13:]
				if int(data[7]) != 5+len(value) || pos+len(value) > len(block) {
					continue
				}
				copy(block[pos:], value)
				reply = []byte("PACKS")
			default:
				continue
			}
			frame := append([]byte("<PACKT><SRCCN>SPAfc:0f:e7:d9:c5:5b</SRCCN><DESCN>client</SRCCN><DESCN>SPAfc:0f:e7:</DESCN><DATAS>"), reply...)
			frame = append(frame, []byte("</DATAS></PACKT>")...)
			pc.WriteTo(frame, addr)
		}
	}()
	return pc.LocalAddr().String()
}

const inYTFiles = "FILES,inYT_C65.xml,inYT_S66.xml"

func TestReadReportsTemperatureSetpointAndHeating(t *testing.T) {
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t)})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 {
		t.Errorf("TemperatureF = %v, want 67.0", snap.TemperatureF)
	}
	if snap.TargetF != 102.0 {
		t.Errorf("TargetF = %v, want 102.0", snap.TargetF)
	}
	if !snap.Heating {
		t.Errorf("Heating = false, want true")
	}
	if snap.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt not set")
	}
}

func TestReadReportsIdleWhenHeaterOff(t *testing.T) {
	status := realStatus(t)
	status[260-256] &^= 0x60 // clear the two heating bits
	addr := serve(t, &fakeModule{files: inYTFiles, status: status})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.Heating {
		t.Errorf("Heating = true, want false")
	}
	if snap.TemperatureF != 67.0 {
		t.Errorf("TemperatureF = %v, want 67.0", snap.TemperatureF)
	}
}

func TestReadRejectsInvalidTemperature(t *testing.T) {
	status := realStatus(t)
	status[279-256] |= 0x04 // TempNotValid
	addr := serve(t, &fakeModule{files: inYTFiles, status: status})

	snap, err := hottub.Read(testCtx(t), addr)
	if err == nil {
		t.Fatalf("expected error when temperature is flagged invalid, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
}

func TestReadRejectsUnsupportedSpaPack(t *testing.T) {
	addr := serve(t, &fakeModule{files: "FILES,inXM_C12.xml,inXM_S34.xml", status: realStatus(t)})

	snap, err := hottub.Read(testCtx(t), addr)
	if err == nil {
		t.Fatalf("expected error for unsupported spa pack, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
}

func TestReadRejectsUnsupportedConfigVersion(t *testing.T) {
	addr := serve(t, &fakeModule{files: "FILES,inYT_C64.xml,inYT_S66.xml", status: realStatus(t)})

	snap, err := hottub.Read(testCtx(t), addr)
	if err == nil {
		t.Fatalf("expected error for unsupported config version, got %+v", snap)
	}
}

func TestReadIsNotConfusedByALateReplyToARetriedRead(t *testing.T) {
	// Status replies carry no request identifier, so a slow answer to the
	// first chunk arriving alongside the resend's answer must not be taken
	// as the second chunk.
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t), slowFirst: true})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 || snap.TargetF != 102.0 || snap.MinTargetF != 59.0 || snap.MaxTargetF != 106.0 {
		t.Errorf("snapshot = %+v, want 67.0/102.0/59.0/106.0", snap)
	}
}

func TestReadReturnsErrorWhenModuleSilent(t *testing.T) {
	addr := serve(t, &fakeModule{silent: true})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	snap, err := hottub.Read(ctx, addr)
	if err == nil {
		t.Fatalf("expected error when module never answers, got %+v", snap)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Read took %v, expected it to honour the context deadline", elapsed)
	}
}

func TestReadRetriesWhenADatagramIsDropped(t *testing.T) {
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t), dropFirst: true})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 || snap.TargetF != 102.0 || !snap.Heating {
		t.Errorf("snapshot = %+v, want 67.0/102.0/heating", snap)
	}
}

func TestReadIgnoresUnsolicitedDatagrams(t *testing.T) {
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t), noisy: true})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 || snap.TargetF != 102.0 || !snap.Heating {
		t.Errorf("snapshot = %+v, want 67.0/102.0/heating", snap)
	}
}

func TestStopReturnsPromptlyWhileModuleIsSilent(t *testing.T) {
	addr := serve(t, &fakeModule{silent: true})
	f := hottub.New(nil)
	f.Start(context.Background(), types.HotTub{Enabled: true, Host: addr}, time.Minute)
	time.Sleep(100 * time.Millisecond) // let the first poll block on the silent module

	start := time.Now()
	f.Stop()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Stop took %v while a poll was pending, want it to cancel promptly", elapsed)
	}
}

func TestReadAcceptsIPv6Literal(t *testing.T) {
	pc, err := net.ListenPacket("udp", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback unavailable")
	}
	pc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = hottub.Read(ctx, "::1")
	if err == nil {
		t.Fatalf("expected timeout from an unanswered address")
	}
	if strings.Contains(err.Error(), "bad host") {
		t.Errorf("bare IPv6 literal rejected as bad host: %v", err)
	}
}

func TestReadReturnsErrorForBadHost(t *testing.T) {
	snap, err := hottub.Read(testCtx(t), "not a host:port:extra")
	if err == nil {
		t.Fatalf("expected error for unparseable host, got %+v", snap)
	}
}

func TestReadReportsSetpointBoundsFromConfig(t *testing.T) {
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t)})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.MinTargetF != 59.0 {
		t.Errorf("MinTargetF = %v, want 59.0", snap.MinTargetF)
	}
	if snap.MaxTargetF != 106.0 {
		t.Errorf("MaxTargetF = %v, want 106.0", snap.MaxTargetF)
	}
}

func TestReadReportsTargetFromUserSetpoint(t *testing.T) {
	// The user-facing setpoint (config SetpointG) is what the arrows change;
	// RealSetPointG in the log block can differ from it under economy mode.
	cfg := realConfig(t)
	binary.BigEndian.PutUint16(cfg[1:], 680) // 100°F
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t), config: cfg})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TargetF != 100.0 {
		t.Errorf("TargetF = %v, want 100.0", snap.TargetF)
	}
}

func TestSetTargetWritesSetpointAndReturnsFreshSnapshot(t *testing.T) {
	m := &fakeModule{files: inYTFiles, status: realStatus(t)}
	addr := serve(t, m)

	snap, err := hottub.SetTarget(testCtx(t), addr, 103)
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if snap.TargetF != 103.0 {
		t.Errorf("TargetF = %v, want 103.0", snap.TargetF)
	}
	if snap.TemperatureF != 67.0 || !snap.Heating {
		t.Errorf("snapshot = %+v, want the rest of the reading intact", snap)
	}

	writes := m.Writes()
	if len(writes) != 1 {
		t.Fatalf("module received %d SPACK commands, want 1: %q", len(writes), writes)
	}
	got := writes[0]
	// SPACK, seq, pack type inYT (10), length 5+2, SET_VALUE (70),
	// config v65, log v66, SetpointG position 1, (103-32)*10 = 710 = 0x02c6.
	want := []byte{'S', 'P', 'A', 'C', 'K', got[5], 10, 7, 70, 65, 66, 0x00, 0x01, 0x02, 0xc6}
	if !bytes.Equal(got, want) {
		t.Errorf("SPACK = % x, want % x", got, want)
	}
}

func TestSetTargetRejectsTargetsOutsideTheTubsRange(t *testing.T) {
	for _, target := range []float64{107, 58, 102.5} {
		m := &fakeModule{files: inYTFiles, status: realStatus(t)}
		addr := serve(t, m)

		snap, err := hottub.SetTarget(testCtx(t), addr, target)
		if err == nil {
			t.Errorf("SetTarget(%v): expected error, got %+v", target, snap)
		}
		if n := len(m.Writes()); n != 0 {
			t.Errorf("SetTarget(%v): module received %d SPACK commands, want 0", target, n)
		}
	}
}

func TestSetTargetReturnsErrorWhenModuleDoesNotAcknowledge(t *testing.T) {
	m := &fakeModule{files: inYTFiles, status: realStatus(t), ignoreWrites: true}
	addr := serve(t, m)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	snap, err := hottub.SetTarget(ctx, addr, 103)
	if err == nil {
		t.Fatalf("expected error when the write is never acknowledged, got %+v", snap)
	}
	if len(m.Writes()) == 0 {
		t.Fatalf("SetTarget failed before sending any SPACK: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("SetTarget took %v, expected it to honour the context deadline", elapsed)
	}
}

func TestFetcherSetTargetPublishesTheNewSnapshot(t *testing.T) {
	addr := serve(t, &fakeModule{files: inYTFiles, status: realStatus(t)})
	var published *types.HotTubSnapshot
	f := hottub.New(func(snap *types.HotTubSnapshot) { published = snap })

	snap, err := f.SetTarget(testCtx(t), types.HotTub{Enabled: true, Host: addr}, 103)
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if snap.TargetF != 103.0 {
		t.Errorf("returned TargetF = %v, want 103.0", snap.TargetF)
	}
	if published == nil || published.TargetF != 103.0 {
		t.Errorf("published snapshot = %+v, want TargetF 103.0", published)
	}
	if cur := f.Snapshot(); cur == nil || cur.TargetF != 103.0 {
		t.Errorf("Snapshot() = %+v, want TargetF 103.0", cur)
	}
}
