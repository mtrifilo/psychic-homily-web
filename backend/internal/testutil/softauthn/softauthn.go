// Package softauthn is a software WebAuthn authenticator for tests. It answers
// the options the server's begin endpoints produce with the JSON a browser
// sends to the finish endpoints, in the WebAuthn Level 3 toJSON() shape that
// @simplewebauthn/browser v13 emits, including the fields the server does not
// read (clientExtensionResults, response.publicKey, publicKeyAlgorithm,
// response.authenticatorData).
package softauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// AAGUID is the authenticator model identifier every Authenticator reports.
var AAGUID = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
)

// Authenticator holds one resident ES256 credential, created by Register.
type Authenticator struct {
	Origin string

	key          *ecdsa.PrivateKey
	credentialID []byte
	userHandle   []byte
	rpID         string
	signCount    uint32
}

// New returns an authenticator that reports the given origin in its client data.
func New(origin string) *Authenticator {
	return &Authenticator{Origin: origin}
}

// UserHandle is the user.id the credential was created for, as the
// authenticator returns it on every assertion.
func (a *Authenticator) UserHandle() []byte { return a.userHandle }

// Register creates a credential for the given creation options and returns
// the RegistrationResponseJSON a browser would send to the finish endpoint.
func (a *Authenticator) Register(creation *protocol.CredentialCreation) ([]byte, error) {
	opts := creation.Response
	handle, err := userHandleBytes(opts.User.ID)
	if err != nil {
		return nil, err
	}

	a.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	a.credentialID = make([]byte, 32)
	if _, err := rand.Read(a.credentialID); err != nil {
		return nil, err
	}
	a.userHandle = handle
	a.rpID = opts.RelyingParty.ID

	coseKey, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{
			KeyType:   int64(webauthncose.EllipticKey),
			Algorithm: int64(webauthncose.AlgES256),
		},
		Curve:  int64(webauthncose.P256),
		XCoord: a.key.X.FillBytes(make([]byte, 32)),
		YCoord: a.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		return nil, err
	}

	authData := a.authenticatorData(flagUserPresent | flagUserVerified | flagAttested)
	authData = append(authData, AAGUID...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(a.credentialID)))
	authData = append(authData, a.credentialID...)
	authData = append(authData, coseKey...)

	attestationObject, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	if err != nil {
		return nil, err
	}

	spki, err := x509.MarshalPKIXPublicKey(&a.key.PublicKey)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"id":                      b64(a.credentialID),
		"rawId":                   b64(a.credentialID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{"credProps": map[string]any{"rk": true}},
		"response": map[string]any{
			"clientDataJSON":     b64(a.clientData("webauthn.create", opts.Challenge)),
			"attestationObject":  b64(attestationObject),
			"authenticatorData":  b64(authData),
			"transports":         []string{"internal"},
			"publicKeyAlgorithm": int(webauthncose.AlgES256),
			"publicKey":          b64(spki),
		},
	})
}

// Assert signs the given request options with the credential Register created
// and returns the AuthenticationResponseJSON a browser would send.
func (a *Authenticator) Assert(assertion *protocol.CredentialAssertion) ([]byte, error) {
	if a.key == nil {
		return nil, fmt.Errorf("softauthn: no credential registered")
	}
	a.signCount++

	authData := a.authenticatorData(flagUserPresent | flagUserVerified)
	clientData := a.clientData("webauthn.get", assertion.Response.Challenge)
	clientDataHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		"id":                      b64(a.credentialID),
		"rawId":                   b64(a.credentialID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"authenticatorData": b64(authData),
			"signature":         b64(signature),
			"userHandle":        b64(a.userHandle),
		},
	})
}

func (a *Authenticator) authenticatorData(flags byte) []byte {
	rpIDHash := sha256.Sum256([]byte(a.rpID))
	data := append([]byte{}, rpIDHash[:]...)
	data = append(data, flags)
	return binary.BigEndian.AppendUint32(data, a.signCount)
}

func (a *Authenticator) clientData(ceremony string, challenge protocol.URLEncodedBase64) []byte {
	data, _ := json.Marshal(map[string]any{
		"type":        ceremony,
		"challenge":   b64(challenge),
		"origin":      a.Origin,
		"crossOrigin": false,
	})
	return data
}

func userHandleBytes(id any) ([]byte, error) {
	switch v := id.(type) {
	case protocol.URLEncodedBase64:
		return v, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("softauthn: unsupported user.id type %T", id)
	}
}

func b64(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
