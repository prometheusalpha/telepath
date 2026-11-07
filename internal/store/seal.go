package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"telepath/internal/model"
)

// Keyring holds the key that seals the workspace's secrets: the system's
// credential store in the app, a map in tests.
type Keyring interface {
	// Get returns the key, or ok false when none is stored yet.
	Get(name string) (value []byte, ok bool, err error)
	Set(name string, value []byte) error
}

const (
	keyName    = "workspace-key"
	sealPrefix = "enc:v1:"
)

// A Sealer encrypts the workspace's secrets in its file: credentials of
// requests' Auth and the values of variables marked secret. One key, in
// the credential store, serves them all, as the store keeps only small
// values.
type Sealer struct {
	aead cipher.AEAD
	// old are keyrings whose keys sealed secrets before, read only for a
	// secret the key can't open: reading another app's item may ask the
	// user, so not at every start. olds are their keys, once read.
	old     []Keyring
	olds    []cipher.AEAD
	oldRead bool
	// stale counts secrets an old key opened, which the next save seals
	// with the key.
	stale int
}

// NewSealer reads the key from kr, making and storing one the first time.
// Secrets the keys of old sealed still open, and are sealed again with
// kr's when saved.
func NewSealer(kr Keyring, old ...Keyring) (*Sealer, error) {
	key, ok, err := kr.Get(keyName)
	if err != nil {
		return nil, err
	}
	if !ok {
		key = make([]byte, 32)
		rand.Read(key)
		if err := kr.Set(keyName, key); err != nil {
			return nil, err
		}
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, old: old}, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("the stored key isn't 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Stale reports whether secrets were opened with an old key, which a save
// would seal with the key.
func (s *Sealer) Stale() bool { return s != nil && s.stale > 0 }

// oldKeys reads the old keyrings' keys the first time they are needed;
// those it can't read are left out.
func (s *Sealer) oldKeys() []cipher.AEAD {
	if !s.oldRead {
		s.oldRead = true
		for _, kr := range s.old {
			if key, ok, err := kr.Get(keyName); err == nil && ok {
				if aead, err := newAEAD(key); err == nil {
					s.olds = append(s.olds, aead)
				}
			}
		}
	}
	return s.olds
}

func (s *Sealer) seal(v string) string {
	if v == "" || strings.HasPrefix(v, sealPrefix) {
		return v
	}
	nonce := make([]byte, s.aead.NonceSize())
	rand.Read(nonce)
	return sealPrefix + base64.RawStdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(v), nil))
}

// open returns v decrypted, or as it is with ok false when it can't be:
// kept sealed, it survives the next save rather than being lost.
func (s *Sealer) open(v string) (string, bool) {
	if !strings.HasPrefix(v, sealPrefix) {
		return v, true
	}
	if s == nil {
		return v, false
	}
	b, err := base64.RawStdEncoding.DecodeString(v[len(sealPrefix):])
	if err != nil {
		return v, false
	}
	if plain, ok := unseal(s.aead, b); ok {
		return plain, true
	}
	for _, aead := range s.oldKeys() {
		if plain, ok := unseal(aead, b); ok {
			s.stale++
			return plain, true
		}
	}
	return v, false
}

func unseal(aead cipher.AEAD, b []byte) (string, bool) {
	n := aead.NonceSize()
	if len(b) < n {
		return "", false
	}
	plain, err := aead.Open(nil, b[:n], b[n:], nil)
	return string(plain), err == nil
}

// secrets calls fn with each secret text of w.
func secrets(w *model.Workspace, fn func(*string)) {
	for i := range w.Env {
		if w.Env[i].Secret {
			fn(&w.Env[i].Value)
		}
	}
	w.Walk(func(n, _ *model.Node) bool {
		if r := n.Request; r != nil {
			fn(&r.Auth.Password)
			fn(&r.Auth.Token)
			fn(&r.Auth.Value)
		}
		return true
	})
}

// sealed returns w as its file holds it, secrets encrypted; w itself is
// left as the app shows it.
func (s *Sealer) sealed(w *model.Workspace) ([]byte, error) {
	b, err := json.Marshal(w)
	if err != nil || s == nil {
		return json.MarshalIndent(w, "", "  ")
	}
	var cp model.Workspace
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	secrets(&cp, func(v *string) { *v = s.seal(*v) })
	return json.MarshalIndent(&cp, "", "  ")
}

// opened decrypts w's secrets in place and counts those it couldn't.
func (s *Sealer) opened(w *model.Workspace) (locked int) {
	secrets(w, func(v *string) {
		var ok bool
		if *v, ok = s.open(*v); !ok {
			locked++
		}
	})
	return locked
}
