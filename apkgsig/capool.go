package apkgsig

import "crypto/x509"

func CACerts() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(pemcerts))
	return pool
}
