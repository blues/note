// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib cert.go contains certificates
package notelib

// GetNotecardSecureElementRootCertificateAsPEM returns a concatenated PEM bundle of ST Microelectronics
// root certificates used to sign the certificate embedded in every Notecard's STSAFE-A100 secure element.
// CA 25 and CA 25_02 (extended expiry) must both be present concurrently to support old and new notecards.
func GetNotecardSecureElementRootCertificateAsPEM() (cert []byte) {
	return []byte(`
-----BEGIN CERTIFICATE-----
MIIB3TCCAWOgAwIBAgIBATAKBggqhkjOPQQDAzBPMQswCQYDVQQGEwJOTDEeMBwG
A1UECgwVU1RNaWNyb2VsZWN0cm9uaWNzIG52MSAwHgYDVQQDDBdTVE0gU1RTQUZF
LUEgUFJPRCBDQSAyNTAeFw0xNzExMTYwMDAwMDBaFw00NzExMTYwMDAwMDBaME8x
CzAJBgNVBAYTAk5MMR4wHAYDVQQKDBVTVE1pY3JvZWxlY3Ryb25pY3MgbnYxIDAe
BgNVBAMMF1NUTSBTVFNBRkUtQSBQUk9EIENBIDI1MHYwEAYHKoZIzj0CAQYFK4EE
ACIDYgAEzJ3UwY415esvvbw/pz2J/UXW6M234YkY4jNCVJieAYMxwtMHDhO1eQke
TAS/WNMz1SHuQ/1gXaS7hfLuk89XJ7dYGsCnEY1GKh6AHVzLo1xk/W6xNNvofoTe
fooVH1sNoxMwETAPBgNVHRMBAf8EBTADAQH/MAoGCCqGSM49BAMDA2gAMGUCMBG2
LhehNFswV9otQG+8RM8BElZHHHH5I40XECvbHu8cVS1m1bmKFG5qhXPp1y1PbgIx
AISPZKUEFntIoVgNkSJO9zi6/RuI25XrikU42sHNVI5YqR2i2PFkaBugfwCYPpxb
4g==
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIB2zCCAWKgAwIBAgIBAjAKBggqhkjOPQQDAzBPMQswCQYDVQQGEwJOTDEeMBwG
A1UECgwVU1RNaWNyb2VsZWN0cm9uaWNzIG52MSAwHgYDVQQDDBdTVE0gU1RTQUZF
LUEgUFJPRCBDQSAyNTAgFw0yNjAzMTIwMDAwMDBaGA8yMDU2MDMxMjAwMDAwMFow
TzELMAkGA1UEBhMCTkwxHjAcBgNVBAoMFVNUTWljcm9lbGVjdHJvbmljcyBudjEg
MB4GA1UEAwwXU1RNIFNUU0FGRS1BIFBST0QgQ0EgMjUwdjAQBgcqhkjOPQIBBgUr
gQQAIgNiAATMndTBjjXl6y+9vD+nPYn9RdbozbfhiRjiM0JUmJ4BgzHC0wcOE7V5
CR5MBL9Y0zPVIe5D/WBdpLuF8u6Tz1cnt1gawKcRjUYqHoAdXMujXGT9brE02+h+
hN5+ihUfWw2jEDAOMAwGA1UdEwQFMAMBAf8wCgYIKoZIzj0EAwMDZwAwZAIwUTFx
JgQ6MconrIeQVwX7nazHOKyC8Dw8dNBU3gpQJdO0gRaY7vORovW5WjUzB7XBAjBm
CPxsVvvE3mWaEE9nXzSLifCNS8FeF2JTEkvp6PzB9grcn79edwBFvKkThuofkbQ=
-----END CERTIFICATE-----
`)
}
