package apkgsig

import "crypto/x509"

// CACerts returns the trusted CA certificate pool for verifying TLS connections
// when downloading packages and databases.
func CACerts() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(pemcerts))
	return pool
}
