package tunnel

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// TestIntegration_RealHandshakeAndDialThrough stands up an in-process SSH server
// (real golang.org/x/crypto/ssh handshake), connects to it via realSSHDialer,
// and dials a target HTTP server THROUGH the SSH connection — asserting bytes
// flow end-to-end and the Manager's DialContext wires into yonner correctly.
//
// This exercises the production SSH adapter and the direct-tcpip channel
// forwarding, complementing the fake-dialer lifecycle tests.
func TestIntegration_RealHandshakeAndDialThrough(t *testing.T) {
	// A target HTTP server the Request will reach THROUGH the bastion.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello through the tunnel")
	}))
	defer target.Close()

	// Stand up the in-process SSH server (password auth, accepts direct-tcpip).
	const password = "s3cret"
	serverAddr, hostPub := startSSHServer(t, "deploy", password)

	host, portStr, _ := net.SplitHostPort(serverAddr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	jh := model.JumpHost{
		Host:     host,
		Port:     port,
		User:     "deploy",
		Auth:     model.JumpAuthPassword,
		Password: password,
	}

	// Trust the server's host key via a fixed callback (the verifier itself is
	// tested separately; here we focus on the real handshake + forwarding).
	hkcb := ssh.FixedHostKey(hostPub)

	m := New(WithSSHDialer(func(ctx context.Context, j model.JumpHost, _ ssh.HostKeyCallback) (SSHConn, error) {
		return realSSHDialer(ctx, j, hkcb)
	}))
	_, release, err := m.Acquire(jh)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	// Send a real HTTP request through the SSH tunnel via yonner.
	req := model.Request{Method: "GET", URL: target.URL}
	opts := yonner.DefaultOptions()
	opts.DialContext = m.DialContext(jh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := yonner.Send(ctx, req, model.Collection{}, opts)
	if err != nil {
		t.Fatalf("Send through tunnel: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if string(resp.Body) != "hello through the tunnel" {
		t.Fatalf("body = %q, want the tunneled greeting", resp.Body)
	}

	// The tunnel should now be Connected with a single underlying connection.
	if st := m.Status(); len(st) != 1 || st[0].State != Connected {
		t.Fatalf("status after send = %+v, want one Connected tunnel", st)
	}
}

// startSSHServer launches a minimal in-process SSH server that authenticates the
// given user/password and forwards "direct-tcpip" channels (the dial-through Yon
// uses). It returns the listen address and the server's host public key.
func startSSHServer(t *testing.T, user, password string) (addr string, hostPub ssh.PublicKey) {
	t.Helper()

	// Host key.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("host key gen: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	hostPub, err = ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("host pub: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("denied")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			nConn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSSHConn(nConn, cfg)
		}
	}()

	return ln.Addr().String(), hostPub
}

// serveSSHConn handles one accepted TCP connection: SSH handshake, then service
// direct-tcpip channel requests by dialing the requested target and splicing.
func serveSSHConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		_ = nConn.Close()
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "direct-tcpip" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only direct-tcpip")
			continue
		}
		go handleDirectTCPIP(newCh)
	}
}

// directTCPIPPayload mirrors the RFC 4254 direct-tcpip channel open payload.
type directTCPIPPayload struct {
	HostToConnect  string
	PortToConnect  uint32
	OriginatorHost string
	OriginatorPort uint32
}

// handleDirectTCPIP accepts a direct-tcpip channel, dials the requested target,
// and copies bytes both ways — the server side of Yon's dial-through.
func handleDirectTCPIP(newCh ssh.NewChannel) {
	var p directTCPIPPayload
	if err := ssh.Unmarshal(newCh.ExtraData(), &p); err != nil {
		_ = newCh.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	target := net.JoinHostPort(p.HostToConnect, fmt.Sprintf("%d", p.PortToConnect))
	dst, err := net.Dial("tcp", target)
	if err != nil {
		_ = newCh.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, chReqs, err := newCh.Accept()
	if err != nil {
		_ = dst.Close()
		return
	}
	go ssh.DiscardRequests(chReqs)

	// Splice channel <-> target until either side closes.
	done := make(chan struct{}, 2)
	cp := func(w io.Writer, r io.Reader) {
		_, _ = io.Copy(w, bufio.NewReader(r))
		done <- struct{}{}
	}
	go cp(ch, dst)
	go cp(dst, ch)
	<-done
	_ = ch.Close()
	_ = dst.Close()
}
