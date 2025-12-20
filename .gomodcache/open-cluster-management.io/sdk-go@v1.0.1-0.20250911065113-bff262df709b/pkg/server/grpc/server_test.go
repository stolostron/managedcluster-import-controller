package grpc

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	certutil "k8s.io/client-go/util/cert"

	cemetrics "open-cluster-management.io/sdk-go/pkg/cloudevents/server/grpc/metrics"
)

// testAuthenticator implements Authenticator for testing
type testAuthenticator struct {
	name string
}

func (a *testAuthenticator) Authenticate(ctx context.Context) (context.Context, error) {
	// Test authentication logic - just return the context unchanged
	return ctx, nil
}

func TestGRPCServerBuilder_Basic(t *testing.T) {
	cert, key, err := certutil.GenerateSelfSignedCertKey("localhost", []net.IP{net.ParseIP("127.0.0.1")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp("", "certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(path)

	err = os.WriteFile(path+"/tls.crt", cert, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path+"/tls.key", key, 0600)
	if err != nil {
		t.Fatal(err)
	}

	opt := NewGRPCServerOptions()
	opt.ClientCAFile = ""
	opt.TLSKeyFile = path + "/tls.key"
	opt.TLSCertFile = path + "/tls.crt"
	opt.ServerBindPort = "0" // Use random port for testing

	builder := NewGRPCServer(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		err := builder.Run(ctx)
		if err != nil {
			t.Errorf("server run error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)
}

func TestGRPCServerBuilder_WithAuthenticatorIntegration(t *testing.T) {
	cert, key, err := certutil.GenerateSelfSignedCertKey("localhost", []net.IP{net.ParseIP("127.0.0.1")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp("", "certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(path)

	err = os.WriteFile(path+"/tls.crt", cert, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path+"/tls.key", key, 0600)
	if err != nil {
		t.Fatal(err)
	}

	opt := NewGRPCServerOptions()
	opt.ClientCAFile = ""
	opt.TLSKeyFile = path + "/tls.key"
	opt.TLSCertFile = path + "/tls.crt"
	opt.ServerBindPort = "0" // Use random port for testing

	// Test chaining authenticators
	builder := NewGRPCServer(opt).
		WithAuthenticator(&testAuthenticator{name: "test1"}).
		WithAuthenticator(&testAuthenticator{name: "test2"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		err := builder.Run(ctx)
		if err != nil {
			t.Errorf("server run error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)
}

func TestGRPCServerBuilder_WithRegisterFunc(t *testing.T) {
	opt := NewGRPCServerOptions()
	builder := NewGRPCServer(opt)

	// Test adding register functions
	var registeredServers []*grpc.Server
	registerFunc1 := func(server *grpc.Server) {
		registeredServers = append(registeredServers, server)
	}
	registerFunc2 := func(server *grpc.Server) {
		registeredServers = append(registeredServers, server)
	}

	result := builder.WithRegisterFunc(registerFunc1)
	if result != builder {
		t.Error("WithRegisterFunc should return the builder for chaining")
	}

	builder.WithRegisterFunc(registerFunc2)

	if len(builder.registerFuncs) != 2 {
		t.Errorf("Expected 2 register functions, got %d", len(builder.registerFuncs))
	}
}

func TestGRPCServerBuilder_WithExtraMetrics(t *testing.T) {
	opt := NewGRPCServerOptions()
	builder := NewGRPCServer(opt)

	// Test add extra metrics
	builder.WithExtraMetrics(cemetrics.CloudEventsGRPCMetrics()...)

	if builder.extraMetrics == nil || len(builder.extraMetrics) != len(cemetrics.CloudEventsGRPCMetrics()) {
		t.Error("Expected extra metrics to be registered")
	}
}

func TestGRPCServerBuilder_WithAuthenticator(t *testing.T) {
	opt := NewGRPCServerOptions()
	builder := NewGRPCServer(opt)

	// Test adding authenticators
	auth1 := &testAuthenticator{name: "auth1"}
	auth2 := &testAuthenticator{name: "auth2"}

	result := builder.WithAuthenticator(auth1)
	if result != builder {
		t.Error("WithAuthenticator should return the builder for chaining")
	}

	builder.WithAuthenticator(auth2)

	if len(builder.authenticators) != 2 {
		t.Errorf("Expected 2 authenticators, got %d", len(builder.authenticators))
	}
}

func TestGRPCServerBuilder_Chaining(t *testing.T) {
	opt := NewGRPCServerOptions()

	registerFunc := func(server *grpc.Server) {
		// Test register function
	}

	// Test method chaining
	builder := NewGRPCServer(opt).
		WithAuthenticator(&testAuthenticator{name: "test1"}).
		WithAuthenticator(&testAuthenticator{name: "test2"}).
		WithRegisterFunc(registerFunc)

	if len(builder.authenticators) != 2 {
		t.Errorf("Expected 2 authenticators, got %d", len(builder.authenticators))
	}

	if len(builder.registerFuncs) != 1 {
		t.Errorf("Expected 1 register function, got %d", len(builder.registerFuncs))
	}
}
