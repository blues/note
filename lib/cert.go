// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib cert.go contains certificates
package notelib

func GetNotecardSecureElementRootCertificateAsPEM() ([]byte cert) {
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
`)
}
