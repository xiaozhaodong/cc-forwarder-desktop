package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestValidateSourcePortRange(t *testing.T) {
	tests := []struct {
		name  string
		start int
		end   int
		valid bool
	}{
		{name: "single port", start: 31080, end: 31080, valid: true},
		{name: "port range", start: 31080, end: 31179, valid: true},
		{name: "privileged port", start: 80, end: 80, valid: false},
		{name: "reversed range", start: 31179, end: 31080, valid: false},
		{name: "too many ports", start: 31080, end: 31336, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSourcePortRange(test.start, test.end)
			if (err == nil) != test.valid {
				t.Fatalf("validateSourcePortRange(%d, %d) error=%v, valid=%v", test.start, test.end, err, test.valid)
			}
		})
	}
}

func TestParseSourcePortRange(t *testing.T) {
	start, end, err := ParseSourcePortRange("31080-31179")
	if err != nil || start != 31080 || end != 31179 {
		t.Fatalf("unexpected parsed range: start=%d end=%d err=%v", start, end, err)
	}
	start, end, err = ParseSourcePortRange("31080")
	if err != nil || start != 31080 || end != 31080 {
		t.Fatalf("unexpected parsed single port: start=%d end=%d err=%v", start, end, err)
	}
	if _, _, err := ParseSourcePortRange("80-90"); err == nil {
		t.Fatal("expected privileged source port range to fail")
	}
}

func TestSourcePortDialContextUsesConfiguredRange(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := sourcePortDialContext(32080, 32179)(ctx, "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer connection.Close()
	localPort := connection.LocalAddr().(*net.TCPAddr).Port
	if localPort < 32080 || localPort > 32179 {
		t.Fatalf("local port %d is outside configured range", localPort)
	}

	select {
	case acceptedConnection := <-accepted:
		_ = acceptedConnection.Close()
	case <-ctx.Done():
		t.Fatal("server did not accept source-port connection")
	}
}
