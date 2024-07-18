// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	autocerterrors "github.com/juju/juju/domain/autocert/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestRetrieveCertX509(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	certUUID := uuid.MustNewUUID().String()
	x509Cert := `
	-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----`

	// Insert a cert.
	_, err := db.Exec(`INSERT INTO autocert_cache VALUES
(?, "cert1", ?, 0)`, certUUID, x509Cert)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	retrievedCertBytes, err := st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(retrievedCertBytes), gc.Equals, x509Cert)
}

func (s *stateSuite) TestRetrieveSpecialChars(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	certUUID := uuid.MustNewUUID().String()
	specialCharsCert := `
	-----BEGIN CERTIFICATE-----
abc123!?$*&()'-=@~;\|/"
-----END CERTIFICATE-----`

	// Insert a cert.
	_, err := db.Exec(`INSERT INTO autocert_cache VALUES
(?, "cert1", ?, 0)`, certUUID, specialCharsCert)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	retrievedCertBytes, err := st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(retrievedCertBytes), gc.Equals, specialCharsCert)
}

func (s *stateSuite) TestRetrieveNoCert(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Retrieve an arbitrary non existent cert.
	_, err := st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIs, autocerterrors.NotFound)
}

func (s *stateSuite) TestInsertX509(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	x509Cert := `
	-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----`

	err := st.Put(context.Background(), "cert1", []byte(x509Cert))
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	row := db.QueryRow("SELECT name, data FROM autocert_cache WHERE name = 'cert1'")
	var (
		name, data string
	)
	err = row.Scan(&name, &data)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "cert1")
	c.Check(data, gc.Equals, x509Cert)
}

func (s *stateSuite) TestInsertSpecialChars(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	specialCharsCert := `
	-----BEGIN CERTIFICATE-----
abc123!?$*&()'-=@~;\|/"
-----END CERTIFICATE-----`

	err := st.Put(context.Background(), "cert1", []byte(specialCharsCert))
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	row := db.QueryRow("SELECT name, data FROM autocert_cache WHERE name = 'cert1'")
	var (
		name, data string
	)
	err = row.Scan(&name, &data)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "cert1")
	c.Check(data, gc.Equals, specialCharsCert)
}

func (s *stateSuite) TestDeleteCertX509(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	certUUID := uuid.MustNewUUID().String()
	x509Cert := `
	-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----`

	// Insert a cert.
	_, err := db.Exec(`INSERT INTO autocert_cache VALUES
(?, "cert1", ?, 0)`, certUUID, x509Cert)
	c.Assert(err, jc.ErrorIsNil)

	// Delete the inserted cert.
	err = st.Delete(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)

	row := db.QueryRow("SELECT name, data FROM autocert_cache WHERE name = 'cert1'")
	var (
		name, data string
	)
	err = row.Scan(&name, &data)
	c.Assert(err, gc.Equals, sql.ErrNoRows)
}

func (s *stateSuite) TestDeleteSpecialChars(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	certUUID := uuid.MustNewUUID().String()
	specialCharsCert := `
	-----BEGIN CERTIFICATE-----
abc123!?$*&()'-=@~;\|/"
-----END CERTIFICATE-----`

	// Insert a cert.
	_, err := db.Exec(`INSERT INTO autocert_cache VALUES
(?, "cert1", ?, 0)`, certUUID, specialCharsCert)
	c.Assert(err, jc.ErrorIsNil)

	// Delete the inserted cert.
	err = st.Delete(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)

	row := db.QueryRow("SELECT name, data FROM autocert_cache WHERE name = 'cert1'")
	var (
		name, data string
	)
	err = row.Scan(&name, &data)
	c.Assert(err, gc.Equals, sql.ErrNoRows)
}

func (s *stateSuite) TestReplaceCert(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Insert one cert.
	specialCharsCert := `
	-----BEGIN CERTIFICATE-----
abc123!?$*&()'-=@~;\|/"
-----END CERTIFICATE-----`
	err := st.Put(context.Background(), "cert1", []byte(specialCharsCert))
	c.Assert(err, jc.ErrorIsNil)

	// Replace the contents of the cert "cert1".
	x509Cert := `
	-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----`

	// Insert a cert.
	err = st.Put(context.Background(), "cert1", []byte(x509Cert))
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	retrievedCertBytes, err := st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(retrievedCertBytes), gc.Equals, x509Cert)
}

func (s *stateSuite) TestFullCRUD(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	x509Cert := `
	-----BEGIN CERTIFICATE-----
MIICEjCCAXsCAg36MA0GCSqGSIb3DQEBBQUAMIGbMQswCQYDVQQGEwJKUDEOMAwG
A1UECBMFVG9reW8xEDAOBgNVBAcTB0NodW8ta3UxETAPBgNVBAoTCEZyYW5rNERE
MRgwFgYDVQQLEw9XZWJDZXJ0IFN1cHBvcnQxGDAWBgNVBAMTD0ZyYW5rNEREIFdl
YiBDQTEjMCEGCSqGSIb3DQEJARYUc3VwcG9ydEBmcmFuazRkZC5jb20wHhcNMTIw
ODIyMDUyNjU0WhcNMTcwODIxMDUyNjU0WjBKMQswCQYDVQQGEwJKUDEOMAwGA1UE
CAwFVG9reW8xETAPBgNVBAoMCEZyYW5rNEREMRgwFgYDVQQDDA93d3cuZXhhbXBs
ZS5jb20wXDANBgkqhkiG9w0BAQEFAANLADBIAkEAm/xmkHmEQrurE/0re/jeFRLl
8ZPjBop7uLHhnia7lQG/5zDtZIUC3RVpqDSwBuw/NTweGyuP+o8AG98HxqxTBwID
AQABMA0GCSqGSIb3DQEBBQUAA4GBABS2TLuBeTPmcaTaUW/LCB2NYOy8GMdzR1mx
8iBIu2H6/E2tiY3RIevV2OW61qY2/XRQg7YPxx3ffeUugX9F4J/iPnnu1zAxxyBy
2VguKv4SWjRFoRkIfIlHX0qVviMhSlNy2ioFLy7JcPZb+v3ftDGywUqcBiVDoea0
Hn+GmxZA
-----END CERTIFICATE-----`

	// Insert a cert.
	err := st.Put(context.Background(), "cert1", []byte(x509Cert))
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the inserted cert.
	retrievedCertBytes, err := st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(retrievedCertBytes), gc.Equals, x509Cert)

	// Delete the inserted cert.
	err = st.Delete(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the non-existent cert.
	_, err = st.Get(context.Background(), "cert1")
	c.Assert(err, jc.ErrorIs, autocerterrors.NotFound)
}
