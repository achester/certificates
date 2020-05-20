package awskms

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"io"

	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/pkg/errors"
	"github.com/smallstep/cli/crypto/pemutil"
)

type Signer struct {
	service   *kms.KMS
	keyID     string
	publicKey crypto.PublicKey
}

// NewSigner creates a new signer using a key in the AWS KMS.
func NewSigner(svc *kms.KMS, signingKey string) (*Signer, error) {
	keyID, err := parseKeyID(signingKey)
	if err != nil {
		return nil, err
	}

	// Make sure that the key exists.
	signer := &Signer{
		service: svc,
		keyID:   keyID,
	}
	if err := signer.preloadKey(keyID); err != nil {
		return nil, err
	}

	return signer, nil
}

func (s *Signer) preloadKey(keyID string) error {
	ctx, cancel := defaultContext()
	defer cancel()

	resp, err := s.service.GetPublicKeyWithContext(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyID,
	})
	if err != nil {
		return errors.Wrap(err, "awskms GetPublicKeyWithContext failed")
	}

	s.publicKey, err = pemutil.ParseDER(resp.PublicKey)
	return err
}

// Public returns the public key of this signer or an error.
func (s *Signer) Public() crypto.PublicKey {
	return s.publicKey
}

// Sign signs digest with the private key stored in the AWS KMS.
func (s *Signer) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	alg, err := getSigningAlgorithm(s.Public(), opts)
	if err != nil {
		return nil, err
	}

	req := &kms.SignInput{
		KeyId:            &s.keyID,
		SigningAlgorithm: &alg,
		Message:          digest,
	}
	req.SetMessageType("DIGEST")

	ctx, cancel := defaultContext()
	defer cancel()

	resp, err := s.service.SignWithContext(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "awsKMS SignWithContext failed")
	}

	return resp.Signature, nil
}

func getSigningAlgorithm(key crypto.PublicKey, opts crypto.SignerOpts) (string, error) {
	switch key.(type) {
	case *rsa.PublicKey:
		_, isPSS := opts.(*rsa.PSSOptions)
		switch h := opts.HashFunc(); h {
		case crypto.SHA256:
			if isPSS {
				return "RSASSA_PSS_SHA_256", nil
			}
			return "RSASSA_PKCS1_V1_5_SHA_256", nil
		case crypto.SHA384:
			if isPSS {
				return "RSASSA_PSS_SHA_384", nil
			}
			return "RSASSA_PKCS1_V1_5_SHA_384", nil
		case crypto.SHA512:
			if isPSS {
				return "RSASSA_PSS_SHA_512", nil
			}
			return "RSASSA_PKCS1_V1_5_SHA_512", nil
		default:
			return "", errors.Errorf("unsupported hash function %v", h)
		}
	case *ecdsa.PublicKey:
		switch h := opts.HashFunc(); h {
		case crypto.SHA256:
			return "ECDSA_SHA_256", nil
		case crypto.SHA384:
			return "ECDSA_SHA_384", nil
		case crypto.SHA512:
			return "ECDSA_SHA_512", nil
		default:
			return "", errors.Errorf("unsupported hash function %v", h)
		}
	default:
		return "", errors.Errorf("unsupported key type %T", key)
	}
}
