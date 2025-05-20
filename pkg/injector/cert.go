package injector

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

func GenerateSelfSignedCert(namespace string) (cert []byte, key []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(4 * 365 * 24 * time.Hour) // valid for 4 years, but this code is re-run on every injector startup

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Infisical"},
			CommonName:   "infisical-agent-injector",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:    []string{"infisical.com", "infisical-agent-injector-svc", "infisical-agent-injector-svc." + namespace, "infisical-agent-injector-svc." + namespace + ".svc"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	// create a tls cert
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// convert to pem format
	certPEM := &bytes.Buffer{}
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// convert private key to pem format
	keyPEM := &bytes.Buffer{}
	pem.Encode(keyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return certPEM.Bytes(), keyPEM.Bytes(), nil
}
