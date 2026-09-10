package hottub_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
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
	silent bool
	// dropFirst ignores the first STATU request so only a resend gets an
	// answer.
	dropFirst bool
	// noisy sends an unsolicited STATP push (which a real module emits to
	// connected clients) before every reply.
	noisy bool
}

// serve runs a fake in.touch2 module on loopback and returns its address. It
// mimics the real module's quirks: only the discovery hello ("1") is answered,
// and the PACKT reply has a mismatched <DESCN>…</SRCCN> tag pair.
func serve(t *testing.T, m fakeModule) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 4096)
		seen := map[string]bool{}
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if m.silent {
				continue
			}
			msg := buf[:n]
			if m.dropFirst && bytes.Contains(msg, []byte("STATU")) && !seen[string(msg)] {
				seen[string(msg)] = true
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
				off := start - 256
				if off < 0 || off+length > len(m.status) {
					continue
				}
				reply = append([]byte("STATV"), data[5], 0, byte(length))
				reply = append(reply, m.status[off:off+length]...)
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
	addr := serve(t, fakeModule{files: inYTFiles, status: realStatus(t)})

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
	addr := serve(t, fakeModule{files: inYTFiles, status: status})

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
	addr := serve(t, fakeModule{files: inYTFiles, status: status})

	snap, err := hottub.Read(testCtx(t), addr)
	if err == nil {
		t.Fatalf("expected error when temperature is flagged invalid, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
}

func TestReadRejectsUnsupportedSpaPack(t *testing.T) {
	addr := serve(t, fakeModule{files: "FILES,inXM_C12.xml,inXM_S34.xml", status: realStatus(t)})

	snap, err := hottub.Read(testCtx(t), addr)
	if err == nil {
		t.Fatalf("expected error for unsupported spa pack, got %+v", snap)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
}

func TestReadReturnsErrorWhenModuleSilent(t *testing.T) {
	addr := serve(t, fakeModule{silent: true})
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
	addr := serve(t, fakeModule{files: inYTFiles, status: realStatus(t), dropFirst: true})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 || snap.TargetF != 102.0 || !snap.Heating {
		t.Errorf("snapshot = %+v, want 67.0/102.0/heating", snap)
	}
}

func TestReadIgnoresUnsolicitedDatagrams(t *testing.T) {
	addr := serve(t, fakeModule{files: inYTFiles, status: realStatus(t), noisy: true})

	snap, err := hottub.Read(testCtx(t), addr)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.TemperatureF != 67.0 || snap.TargetF != 102.0 || !snap.Heating {
		t.Errorf("snapshot = %+v, want 67.0/102.0/heating", snap)
	}
}

func TestStopReturnsPromptlyWhileModuleIsSilent(t *testing.T) {
	addr := serve(t, fakeModule{silent: true})
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
