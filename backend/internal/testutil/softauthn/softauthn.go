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
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// AAGUID is the authenticator model identifier every Authenticator reports.
var AAGUID = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

// Authenticator holds one resident ES256 credential, created by Register.
type Authenticator struct {
	Origin string
	// BackupEligible and BackedUp set the BE and BS flags of every ceremony.
	BackupEligible bool
	BackedUp       bool

	key          *ecdsa.PrivateKey
	credentialID protocol.URLEncodedBase64
	userHandle   protocol.URLEncodedBase64
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
	if _, ok := creation.Response.User.ID.(protocol.URLEncodedBase64); !ok {
		return nil, fmt.Errorf("softauthn: unsupported user.id type %T", creation.Response.User.ID)
	}

	var err error
	a.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	a.credentialID = make([]byte, 32)
	if _, err := rand.Read(a.credentialID); err != nil {
		return nil, err
	}
	return a.attest(creation)
}

// RegisterAgain answers the given creation options with the credential
// Register created, keeping its id and key, as an authenticator that ignores
// excludeCredentials does.
func (a *Authenticator) RegisterAgain(creation *protocol.CredentialCreation) ([]byte, error) {
	if a.key == nil {
		return nil, fmt.Errorf("softauthn: no credential registered")
	}
	return a.attest(creation)
}

// attest returns the RegistrationResponseJSON for the held credential,
// created for the user and relying party the creation options name.
func (a *Authenticator) attest(creation *protocol.CredentialCreation) ([]byte, error) {
	opts := creation.Response
	handle, ok := opts.User.ID.(protocol.URLEncodedBase64)
	if !ok {
		return nil, fmt.Errorf("softauthn: unsupported user.id type %T", opts.User.ID)
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

	authData := a.authenticatorData(protocol.FlagUserPresent | protocol.FlagUserVerified | protocol.FlagAttestedCredentialData)
	authData = append(authData, AAGUID...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(a.credentialID)))
	authData = append(authData, a.credentialID...)
	authData = append(authData, coseKey...)

	attestationObject, err := webauthncbor.Marshal(map[string]any{
		"fmt":      string(protocol.AttestationFormatNone),
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

	return a.credentialJSON(map[string]any{"credProps": map[string]any{"rk": true}}, map[string]any{
		"clientDataJSON":     a.clientData(protocol.CreateCeremony, opts.Challenge),
		"attestationObject":  protocol.URLEncodedBase64(attestationObject),
		"authenticatorData":  protocol.URLEncodedBase64(authData),
		"transports":         []string{"internal"},
		"publicKeyAlgorithm": int(webauthncose.AlgES256),
		"publicKey":          protocol.URLEncodedBase64(spki),
	})
}

// Assert signs the given request options with the credential Register created
// and returns the AuthenticationResponseJSON a browser would send.
func (a *Authenticator) Assert(assertion *protocol.CredentialAssertion) ([]byte, error) {
	if a.key == nil {
		return nil, fmt.Errorf("softauthn: no credential registered")
	}
	a.signCount++

	authData := a.authenticatorData(protocol.FlagUserPresent | protocol.FlagUserVerified)
	clientData := a.clientData(protocol.AssertCeremony, assertion.Response.Challenge)
	clientDataHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		return nil, err
	}

	return a.credentialJSON(map[string]any{}, map[string]any{
		"clientDataJSON":    clientData,
		"authenticatorData": protocol.URLEncodedBase64(authData),
		"signature":         protocol.URLEncodedBase64(signature),
		"userHandle":        a.userHandle,
	})
}

// credentialJSON wraps a ceremony's response in the PublicKeyCredential
// fields both ceremonies share.
func (a *Authenticator) credentialJSON(extensions, response map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":                      a.credentialID,
		"rawId":                   a.credentialID,
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  extensions,
		"response":                response,
	})
}

func (a *Authenticator) authenticatorData(flags protocol.AuthenticatorFlags) []byte {
	if a.BackupEligible {
		flags |= protocol.FlagBackupEligible
	}
	if a.BackedUp {
		flags |= protocol.FlagBackupState
	}
	rpIDHash := sha256.Sum256([]byte(a.rpID))
	data := append([]byte{}, rpIDHash[:]...)
	data = append(data, byte(flags))
	return binary.BigEndian.AppendUint32(data, a.signCount)
}

func (a *Authenticator) clientData(ceremony protocol.CeremonyType, challenge protocol.URLEncodedBase64) protocol.URLEncodedBase64 {
	data, _ := json.Marshal(protocol.CollectedClientData{
		Type:      ceremony,
		Challenge: challenge.String(),
		Origin:    a.Origin,
	})
	return data
}
