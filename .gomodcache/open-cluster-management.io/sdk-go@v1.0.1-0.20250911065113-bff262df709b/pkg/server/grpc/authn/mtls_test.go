package authn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestMtlsAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		authInfo credentials.TLSInfo
		valid    bool
	}{
		{
			name:     "no info",
			authInfo: credentials.TLSInfo{},
			valid:    false,
		},
		{
			name: "nil chain",
			authInfo: credentials.TLSInfo{
				State: tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{nil},
				},
			},
			valid: false,
		},
		{
			name: "valid chain",
			authInfo: credentials.TLSInfo{
				State: tls.ConnectionState{
					VerifiedChains: [][]*x509.Certificate{
						{
							{
								Subject: pkix.Name{},
							},
						},
					},
				},
			},
			valid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &peer.Peer{
				AuthInfo: test.authInfo,
			}
			ctx := peer.NewContext(context.Background(), p)
			authenticator := MtlsAuthenticator{}
			_, err := authenticator.Authenticate(ctx)
			if test.valid && err != nil {
				t.Errorf("authenticator.Authenticate() = %v", err)
			} else if !test.valid && err == nil {
				t.Errorf("authenticator.Authenticate() = %v, wanted error", err)
			}
		})
	}
}
