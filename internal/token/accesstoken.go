// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package token vendors the VRTC AccessToken algorithm and adds
// the issue/check/inspect helpers used by the `token` commands and by doctor.
//
// The AccessToken portion below is ported from the official VRTC Go reference
// implementation. Wire format is unchanged so tokens interoperate with the RTC
// service:
//
//	"001" + AppID(24) + base64( pack(msg) + pack(sig) )
//	sig = HMAC-SHA256(msg, AppKey)
//
// All signing is local; no network calls.
package token

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	version       = "001"
	versionLength = 3
	appIDLength   = 24
)

// Privilege enumerates token permissions (wire values must not change).
type Privilege uint16

const (
	PrivPublishStream Privilege = iota
	privPublishAudioStream
	privPublishVideoStream
	privPublishDataStream
	PrivSubscribeStream
)

// AccessToken is the VRTC token structure.
type AccessToken struct {
	AppID      string
	AppKey     string
	RoomID     string
	UserID     string
	IssuedAt   uint32
	ExpireAt   uint32
	Nonce      uint32
	Privileges map[uint16]uint32
	Signature  string
}

// NewAccessToken initializes a token with required parameters.
func NewAccessToken(appID, appKey, roomID, userID string) *AccessToken {
	return &AccessToken{
		AppID:      appID,
		AppKey:     appKey,
		RoomID:     roomID,
		UserID:     userID,
		IssuedAt:   uint32(time.Now().Unix()),
		Nonce:      rand.Uint32N(99999999) + 1,
		Privileges: make(map[uint16]uint32),
	}
}

// ParseAccessToken retrieves token information from a raw string.
func ParseAccessToken(raw string) (*AccessToken, error) {
	if len(raw) <= versionLength+appIDLength {
		return nil, fmt.Errorf("invalid token length: %d", len(raw))
	}
	if raw[:versionLength] != version {
		return nil, fmt.Errorf("expect version %s, got %s", version, raw[:versionLength])
	}

	t := new(AccessToken)
	t.AppID = raw[versionLength : versionLength+appIDLength]

	contentEncoded := raw[versionLength+appIDLength:]
	content, err := base64.StdEncoding.DecodeString(contentEncoded)
	if err != nil {
		return nil, errors.New("failed to base64-decode token content")
	}
	msg, signature, err := unpackContent(content)
	if err != nil {
		return nil, errors.New("failed to unpack token content")
	}
	t.Signature = signature

	in := bytes.NewReader([]byte(msg))
	t.Privileges = make(map[uint16]uint32)
	if t.Nonce, err = unpackUint32(in); err != nil {
		return nil, errors.New("failed to unpack nonce")
	}
	if t.IssuedAt, err = unpackUint32(in); err != nil {
		return nil, errors.New("failed to unpack issuedAt")
	}
	if t.ExpireAt, err = unpackUint32(in); err != nil {
		return nil, errors.New("failed to unpack expireAt")
	}
	if t.RoomID, err = unpackString(in); err != nil {
		return nil, errors.New("failed to unpack roomID")
	}
	if t.UserID, err = unpackString(in); err != nil {
		return nil, errors.New("failed to unpack userID")
	}
	keyLength, err := unpackUint16(in)
	if err != nil {
		return nil, errors.New("failed to unpack privilege count")
	}
	for i := uint16(0); i < keyLength; i++ {
		key, err := unpackUint16(in)
		if err != nil {
			return nil, errors.New("failed to unpack privilege key")
		}
		value, err := unpackUint32(in)
		if err != nil {
			return nil, errors.New("failed to unpack privilege value")
		}
		t.Privileges[key] = value
	}
	return t, nil
}

// Verify checks the token signature and global expiry against key.
func (t *AccessToken) Verify(key string) bool {
	if t.ExpireAt > 0 && uint32(time.Now().Unix()) > t.ExpireAt {
		return false
	}
	t.AppKey = key
	_, sign, err := t.pack()
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(sign), []byte(t.Signature))
}

// AddPrivilege grants a permission with an expiration (zero = never expires).
func (t *AccessToken) AddPrivilege(p Privilege, expireAt time.Time) {
	if t.Privileges == nil {
		t.Privileges = make(map[uint16]uint32)
	}
	expire := uint32(expireAt.Unix())
	if expireAt.IsZero() {
		expire = 0
	}
	t.Privileges[uint16(p)] = expire
	if p == PrivPublishStream {
		t.Privileges[uint16(privPublishVideoStream)] = expire
		t.Privileges[uint16(privPublishAudioStream)] = expire
		t.Privileges[uint16(privPublishDataStream)] = expire
	}
}

// ExpireTime sets the global token expiry (zero = never expires).
func (t *AccessToken) ExpireTime(et time.Time) {
	if !et.IsZero() {
		t.ExpireAt = uint32(et.Unix())
	}
}

// Serialize produces the token string.
func (t *AccessToken) Serialize() (string, error) {
	msg, sign, err := t.pack()
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	if err := packString(buf, msg); err != nil {
		return "", err
	}
	if err := packString(buf, sign); err != nil {
		return "", err
	}
	return version + t.AppID + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func (t *AccessToken) pack() (string, string, error) {
	buf := new(bytes.Buffer)
	for _, w := range []func() error{
		func() error { return packUint32(buf, t.Nonce) },
		func() error { return packUint32(buf, t.IssuedAt) },
		func() error { return packUint32(buf, t.ExpireAt) },
		func() error { return packString(buf, t.RoomID) },
		func() error { return packString(buf, t.UserID) },
		func() error { return packMapUint32(buf, t.Privileges) },
	} {
		if err := w(); err != nil {
			return "", "", err
		}
	}
	msg := buf.Bytes()
	mac := hmac.New(sha256.New, []byte(t.AppKey))
	mac.Write(msg)
	return string(msg), string(mac.Sum(nil)), nil
}

// --- binary pack/unpack helpers (little-endian, wire-compatible) ---

func packUint16(w io.Writer, n uint16) error { return binary.Write(w, binary.LittleEndian, n) }
func packUint32(w io.Writer, n uint32) error { return binary.Write(w, binary.LittleEndian, n) }

func packString(w io.Writer, s string) error {
	if err := packUint16(w, uint16(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

func packMapUint32(w io.Writer, m map[uint16]uint32) error {
	if err := packUint16(w, uint16(len(m))); err != nil {
		return err
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		if err := packUint16(w, uint16(k)); err != nil {
			return err
		}
		if err := packUint32(w, m[uint16(k)]); err != nil {
			return err
		}
	}
	return nil
}

func unpackUint16(r io.Reader) (uint16, error) {
	var n uint16
	err := binary.Read(r, binary.LittleEndian, &n)
	return n, err
}

func unpackUint32(r io.Reader) (uint32, error) {
	var n uint32
	err := binary.Read(r, binary.LittleEndian, &n)
	return n, err
}

func unpackString(r io.Reader) (string, error) {
	n, err := unpackUint16(r)
	if err != nil {
		return "", err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func unpackContent(b []byte) (msg, sig string, err error) {
	in := bytes.NewReader(b)
	if msg, err = unpackString(in); err != nil {
		return "", "", err
	}
	if sig, err = unpackString(in); err != nil {
		return "", "", err
	}
	return msg, sig, nil
}
