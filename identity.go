// Copyright (c) 2021, ZeroTier, Inc.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package ztidentity

import (
	secrand "crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/salsa20/salsa"
)

const ztIdentityGenMemory = 2097152
const ztIdentityHashCashFirstByteLessThan = 17

func computeZeroTierIdentityMemoryHardHash(publicKey []byte) []byte {
	s512 := sha512.Sum512(publicKey)

	var genmem [ztIdentityGenMemory]byte
	var s20key [32]byte
	var s20ctr [16]byte
	var s20ctri uint64
	copy(s20key[:], s512[0:32])
	copy(s20ctr[0:8], s512[32:40])
	salsa.XORKeyStream(genmem[0:64], genmem[0:64], &s20ctr, &s20key)
	s20ctri++
	for i := 64; i < ztIdentityGenMemory; i += 64 {
		binary.LittleEndian.PutUint64(s20ctr[8:16], s20ctri)
		salsa.XORKeyStream(genmem[i:i+64], genmem[i-64:i], &s20ctr, &s20key)
		s20ctri++
	}

	var tmp [8]byte
	for i := 0; i < ztIdentityGenMemory; {
		idx1 := uint(binary.BigEndian.Uint64(genmem[i:])&7) * 8
		i += 8
		idx2 := (uint(binary.BigEndian.Uint64(genmem[i:])) % uint(ztIdentityGenMemory/8)) * 8
		i += 8
		gm := genmem[idx2 : idx2+8]
		d := s512[idx1 : idx1+8]
		copy(tmp[:], gm)
		copy(gm, d)
		copy(d, tmp[:])
		binary.LittleEndian.PutUint64(s20ctr[8:16], s20ctri)
		salsa.XORKeyStream(s512[:], s512[:], &s20ctr, &s20key)
		s20ctri++
	}

	return s512[:]
}

// generateDualPair generates a key pair containing two pairs: one for curve25519 and one for ed25519.
func generateDualPair() (pub [64]byte, priv [64]byte) {
	k0pub, k0priv, _ := ed25519.GenerateKey(secrand.Reader)
	var k1pub, k1priv [32]byte
	_, err := io.ReadFull(secrand.Reader, k1priv[:])
	if err != nil {
		panic(fmt.Sprintf("Not enough entropy: %v", err)) // FIXME for now; will adjust prototypes later
	}
	curve25519.ScalarBaseMult(&k1pub, &k1priv)
	copy(pub[0:32], k1pub[:])
	copy(pub[32:64], k0pub[0:32])
	copy(priv[0:32], k1priv[:])
	copy(priv[32:64], k0priv[0:32])
	return
}

// ZeroTierIdentity contains a public key, a private key, and a string representation of the identity.
type ZeroTierIdentity struct {
	address    uint64 // ZeroTier address, only least significant 40 bits are used
	publicKey  [64]byte
	privateKey *[64]byte
}

// NewZeroTierIdentity creates a new ZeroTier Identity.
// This can be a little bit time-consuming due to one way proof of work requirements (usually a few hundred milliseconds).
func NewZeroTierIdentity() (id ZeroTierIdentity) {
	for {
		pub, priv := generateDualPair()
		dig := computeZeroTierIdentityMemoryHardHash(pub[:])
		if dig[0] < ztIdentityHashCashFirstByteLessThan && dig[59] != 0xff {
			id.address = uint64(dig[59])
			id.address <<= 8
			id.address |= uint64(dig[60])
			id.address <<= 8
			id.address |= uint64(dig[61])
			id.address <<= 8
			id.address |= uint64(dig[62])
			id.address <<= 8
			id.address |= uint64(dig[63])
			if id.address != 0 {
				id.publicKey = pub
				id.privateKey = &priv
				break
			}
		}
	}
	return
}

// PrivateKeyString returns the full identity.secret if the private key is set, or an empty string if no private key is set.
func (id *ZeroTierIdentity) PrivateKeyString() string {
	if id.privateKey != nil {
		return fmt.Sprintf("%.10x:0:%x:%x", id.address, id.publicKey, *id.privateKey)
	}
	return ""
}

// PublicKeyString returns identity.public contents.
func (id *ZeroTierIdentity) PublicKeyString() string {
	return fmt.Sprintf("%.10x:0:%x", id.address, id.publicKey)
}

// IDString returns the NodeID as a 10-digit hex string
func (id *ZeroTierIdentity) IDString() string {
	return fmt.Sprintf("%.10x", id.address)
}

// ID returns the ZeroTier address as a uint64
func (id *ZeroTierIdentity) ID() uint64 {
	return id.address
}

// PrivateKey returns the bytes of the private key (or nil if not set)
func (id *ZeroTierIdentity) PrivateKey() *[64]byte {
	return id.privateKey
}

// PublicKey returns the public key bytes
func (id *ZeroTierIdentity) PublicKey() [64]byte {
	return id.publicKey
}
