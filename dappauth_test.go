package dappauth

import (
	"crypto/ecdsa"
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethCrypto "github.com/ethereum/go-ethereum/crypto"
)

type dappauthTest struct {
	title                         string // the test's title
	isEOA                         bool
	challenge                     string
	challengeSign                 string // challengeSign should always equal to challenge unless testing for specific key recovery logic
	signingKeys                   []*ecdsa.PrivateKey
	authAddr                      common.Address
	mockContract                  *mockContract
	expectedAuthorizedSignerError bool
	expectedAuthorizedSigner      bool
}

func TestDappAuth(t *testing.T) {

	keyA, err := ethCrypto.GenerateKey()
	checkError(err, t)
	keyB, err := ethCrypto.GenerateKey()
	checkError(err, t)
	keyC, err := ethCrypto.GenerateKey()
	checkError(err, t)

	dappauthTests := []dappauthTest{
		{
			title:         "External wallets should be authorized signers over their address",
			isEOA:         true,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyA},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               [20]byte{},
				authorizedKey:         nil,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      true,
		},
		{
			title:         "External wallets should NOT be authorized signers when signing the wrong challenge",
			isEOA:         true,
			challenge:     "foo",
			challengeSign: "bar",
			signingKeys:   []*ecdsa.PrivateKey{keyA},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               [20]byte{},
				authorizedKey:         nil,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      false,
		},
		{
			title:         "External wallets should NOT be authorized signers over OTHER addresses",
			isEOA:         true,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyA},
			authAddr:      ethCrypto.PubkeyToAddress(keyB.PublicKey),
			mockContract: &mockContract{
				address:               [20]byte{},
				authorizedKey:         nil,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      false,
		},
		{
			title:         "Smart-contract wallets with a 1-of-1 correct internal key should be authorized signers over their address",
			isEOA:         false,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyB},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               ethCrypto.PubkeyToAddress(keyA.PublicKey),
				authorizedKey:         &keyB.PublicKey,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      true,
		},
		{
			title:         "Smart-contract wallets with a 1-of-2 correct internal key should be authorized signers over their address",
			isEOA:         false,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyB, keyC},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               ethCrypto.PubkeyToAddress(keyA.PublicKey),
				authorizedKey:         &keyB.PublicKey,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      true,
		},
		{
			title:         "Smart-contract wallets with a 1-of-1 incorrect internal key should NOT be authorized signers over their address",
			isEOA:         false,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyB},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               ethCrypto.PubkeyToAddress(keyA.PublicKey),
				authorizedKey:         &keyC.PublicKey,
				errorIsValidSignature: false,
			},
			expectedAuthorizedSignerError: false,
			expectedAuthorizedSigner:      false,
		},
		{
			title:         "IsAuthorizedSigner should error when smart-contract call errors",
			isEOA:         false,
			challenge:     "foo",
			challengeSign: "foo",
			signingKeys:   []*ecdsa.PrivateKey{keyB},
			authAddr:      ethCrypto.PubkeyToAddress(keyA.PublicKey),
			mockContract: &mockContract{
				address:               ethCrypto.PubkeyToAddress(keyA.PublicKey),
				authorizedKey:         &keyB.PublicKey,
				errorIsValidSignature: true,
			},
			expectedAuthorizedSignerError: true,
			expectedAuthorizedSigner:      false,
		},
	}

	// iterate over main test cases
	for _, test := range dappauthTests {
		t.Run(test.title, func(t *testing.T) {
			authenticator := NewAuthenticator(nil, test.mockContract)

			var sig string
			for _, signingKey := range test.signingKeys {
				sig += generateSignature(test.isEOA, test.challengeSign, signingKey, test.authAddr, t)
			}
			isAuthorizedSigner, err := authenticator.IsAuthorizedSigner(test.challenge, sig, test.authAddr.Hex())

			expectBool(err != nil, test.expectedAuthorizedSignerError, t)
			expectBool(isAuthorizedSigner, test.expectedAuthorizedSigner, t)
		})
	}

}

func generateSignature(isEOA bool, msg string, key *ecdsa.PrivateKey, address common.Address, t *testing.T) string {
	if isEOA {
		return signEOAPersonalMessage(msg, key, t)
	}
	return signERC1654PersonalMessage(msg, key, address, t)
}

// emulates what EOA wallets like MetaMask perform
func signEOAPersonalMessage(msg string, key *ecdsa.PrivateKey, t *testing.T) string {
	ethMsgHash := personalMessageHash(msg)
	sig, err := ethCrypto.Sign(ethMsgHash, key)
	checkError(err, t)

	sig[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	return hex.EncodeToString(sig)
}

func signERC1654PersonalMessage(msg string, key *ecdsa.PrivateKey, address common.Address, t *testing.T) string {

	// we hash once before erc191MessageHash as it will be transmitted to Ethereum nodes and potentially logged
	msgHash := erc191MessageHash(ethCrypto.Keccak256([]byte(msg)), address)

	sig, err := ethCrypto.Sign(msgHash, key)
	checkError(err, t)

	sig[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	return hex.EncodeToString(sig)
}

func checkError(err error, t *testing.T) {
	if err != nil {
		t.Error(err)
	}
}

func expectBool(actual, expected bool, t *testing.T) {
	if actual != expected {
		t.Errorf("expected %v to be %v", actual, expected)
	}
}
