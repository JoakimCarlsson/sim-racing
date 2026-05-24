//go:build generate_golden

package protocol_test

import (
	"os"
	"testing"

	"github.com/JoakimCarlsson/sim-racing/internal/physics"
	"github.com/JoakimCarlsson/sim-racing/internal/protocol"
)

func TestGenerateGolden(t *testing.T) {
	// ClientInput
	ci := fixedClientInput()
	ciBuf := make([]byte, protocol.ClientInputSize)
	n, err := ci.Marshal(ciBuf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/golden_client_input.bin", ciBuf[:n], 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("ClientInput: %d bytes", n)

	// ServerSnapshot
	snap := fixedServerSnapshot()
	snapBuf := make([]byte, protocol.ServerSnapshotSize(len(snap.Cars)))
	n, err = snap.Marshal(snapBuf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/golden_server_snapshot.bin", snapBuf[:n], 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("ServerSnapshot: %d bytes", n)

	// ServerHello
	hello := fixedServerHello()
	size := protocol.ServerHelloSize(&hello.Constants)
	helloBuf := make([]byte, size)
	n, err = hello.Marshal(helloBuf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/golden_server_hello.bin", helloBuf[:n], 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("ServerHello: %d bytes", n)

	_ = physics.DefaultConstants // ensure import used
}
