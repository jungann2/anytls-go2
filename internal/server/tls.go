package server

import (
	"anytls/internal/config"
	"anytls/util"
	"crypto/tls"
	"time"

	"github.com/sirupsen/logrus"
)

// LoadTLSConfig loads TLS configuration from the config.
// If external cert/key files are specified and valid, they are used.
// Otherwise, falls back to a self-signed certificate via util.GenerateKeyPair.
// MinVersion is always set to TLS 1.2.
func LoadTLSConfig(cfg *config.Config) (*tls.Config, error) {
	var cert *tls.Certificate

	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		loaded, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			logrus.Warnf("failed to load TLS certificate from %s and %s: %v, falling back to self-signed certificate",
				cfg.TLS.CertFile, cfg.TLS.KeyFile, err)
		} else {
			cert = &loaded
			logrus.Infof("loaded TLS certificate from %s", cfg.TLS.CertFile)
		}
	}

	if cert == nil {
		generated, err := util.GenerateKeyPair(time.Now, "")
		if err != nil {
			return nil, err
		}
		cert = generated
		logrus.Warn("using self-signed certificate")
	}

	tlsCert := cert
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return tlsCert, nil
		},
	}

	return tlsConfig, nil
}
