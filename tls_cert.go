package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	certFile = "cert.pem"
	keyFile  = "key.pem"
)

// LoadOrCreateTLSConfig 加载已有证书，或首次自动生成自签证书并保存。
// dataDir 为空时默认使用 ~/.163-wrapper/。
func LoadOrCreateTLSConfig(dataDir string) (*tls.Config, string, error) {
	var dir string
	if dataDir != "" {
		dir = dataDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		dir = filepath.Join(home, ".163-wrapper")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, "", err
	}
	cp := filepath.Join(dir, certFile)
	kp := filepath.Join(dir, keyFile)

	if _, err := os.Stat(cp); os.IsNotExist(err) {
		if err := generateSelfSigned(cp, kp); err != nil {
			return nil, "", err
		}
	}

	cert, err := tls.LoadX509KeyPair(cp, kp)
	if err != nil {
		return nil, "", err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, cp, nil
}

func generateSelfSigned(certPath, keyPath string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "163-wrapper local proxy",
			Organization: []string{"163-wrapper"},
		},
		DNSNames:  []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(10 * 365 * 24 * time.Hour), // 10 年
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	cf, err := os.OpenFile(certPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer cf.Close()
	if err := pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	kf, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer kf.Close()
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	return pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
}
