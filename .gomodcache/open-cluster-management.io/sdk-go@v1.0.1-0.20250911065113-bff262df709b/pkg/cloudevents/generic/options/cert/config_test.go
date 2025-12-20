package cert

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

func TestCertConfig(t *testing.T) {
	cases := []struct {
		name   string
		config string
	}{
		{
			name: "yaml config",
			config: `
caFile: ca.crt
clientCertFile: client.crt
clientKeyFile: client.key
caData: Q0EgRGF0YQ==
clientCertData: Q2xpZW50IENlcnQgRGF0YQ==
clientKeyData: Q2xpZW50IEtleSBEYXRh
`,
		},
		{
			name: "json config",
			config: `{
"caFile": "ca.crt",
"clientCertFile": "client.crt",
"clientKeyFile": "client.key",
"caData": "Q0EgRGF0YQ==",
"clientCertData": "Q2xpZW50IENlcnQgRGF0YQ==",
"clientKeyData": "Q2xpZW50IEtleSBEYXRh"
}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var config CertConfig
			err := yaml.Unmarshal([]byte(c.config), &config)
			assert.NoError(t, err)
			assert.Equal(t, config.CAFile, "ca.crt")
			assert.Equal(t, config.ClientCertFile, "client.crt")
			assert.Equal(t, config.ClientKeyFile, "client.key")
			assert.Equal(t, config.CAData, Bytes("CA Data"))
			assert.Equal(t, config.ClientCertData, Bytes("Client Cert Data"))
			assert.Equal(t, config.ClientKeyData, Bytes("Client Key Data"))

			output, err := yaml.Marshal(&config)
			assert.NoError(t, err)

			var outputConfig CertConfig
			err = yaml.Unmarshal(output, &outputConfig)
			assert.NoError(t, err)
			assert.Equal(t, config, outputConfig)
		})
	}
}

func TestEmbedCerts(t *testing.T) {
	dir := t.TempDir()

	caFile := filepath.Join(dir, "ca.crt")
	certFile := filepath.Join(dir, "client.crt")
	keyFile := filepath.Join(dir, "client.key")

	if err := os.WriteFile(caFile, []byte("CA Data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, []byte("Client Cert Data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("Client Key Data"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		config CertConfig
	}{
		{
			name: "only cert files set",
			config: CertConfig{
				CAFile:         caFile,
				ClientCertFile: certFile,
				ClientKeyFile:  keyFile,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.config.EmbedCerts()
			assert.NoError(t, err)
			assert.Equal(t, c.config.CAData, Bytes("CA Data"))
			assert.Equal(t, c.config.ClientCertData, Bytes("Client Cert Data"))
			assert.Equal(t, c.config.ClientKeyData, Bytes("Client Key Data"))
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		config CertConfig
		valid  bool
	}{
		{
			name: "both client cert and key set",
			config: CertConfig{
				ClientCertData: Bytes("cert"),
				ClientKeyData:  Bytes("key"),
			},
			valid: true,
		},
		{
			name: "only client cert set",
			config: CertConfig{
				ClientCertData: Bytes("cert"),
			},
			valid: false,
		},
		{
			name: "only client key set",
			config: CertConfig{
				ClientKeyData: Bytes("key"),
			},
			valid: false,
		},
		{
			name: "only ca",
			config: CertConfig{
				CAData: Bytes("ca"),
			},
			valid: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.config.Validate()
			if c.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestHasCerts(t *testing.T) {
	cases := []struct {
		name   string
		config CertConfig
		expect bool
	}{
		{
			name: "has both cert and key",
			config: CertConfig{
				ClientCertData: []byte("cert"),
				ClientKeyData:  []byte("key"),
			},
			expect: true,
		},
		{
			name: "only cert",
			config: CertConfig{
				ClientCertData: []byte("cert"),
			},
			expect: false,
		},
		{
			name:   "no cert or key",
			config: CertConfig{},
			expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.config.HasCerts()
			assert.Equal(t, c.expect, got)
		})
	}
}
