package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenClaims represents Kubernetes service account token claims
type TokenClaims struct {
	jwt.RegisteredClaims
	Kubernetes KubernetesClaims `json:"kubernetes.io"`
}

// KubernetesClaims represents the kubernetes.io nested claims
type KubernetesClaims struct {
	Namespace      string             `json:"namespace"`
	ServiceAccount ServiceAccountInfo `json:"serviceaccount"`
}

// ServiceAccountInfo represents service account details in the token
type ServiceAccountInfo struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// JWTSigner generates and signs Kubernetes service account tokens
type JWTSigner struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
}

// NewJWTSigner creates a new JWT signer with a generated RSA key pair
func NewJWTSigner(issuer string) (*JWTSigner, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return &JWTSigner{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		issuer:     issuer,
	}, nil
}

// GenerateToken creates a valid Kubernetes service account JWT
func (s *JWTSigner) GenerateToken(namespace, name, uid string, audiences []string, expiration time.Time) (string, error) {
	now := time.Now()

	// Generate unique JWT ID (jti) - use UUID format
	jti := uuid.New().String()

	claims := TokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   "system:serviceaccount:" + namespace + ":" + name,
			Audience:  audiences,
			ExpiresAt: jwt.NewNumericDate(expiration),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti, // JWT ID - unique for each token
		},
		Kubernetes: KubernetesClaims{
			Namespace: namespace,
			ServiceAccount: ServiceAccountInfo{
				Name: name,
				UID:  uid,
			},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// ParseToken parses and validates a JWT token
func (s *JWTSigner) ParseToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.publicKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}
