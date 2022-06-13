package data

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"
)

var randomStringUnique = sync.Map{}

func init() {
	var s int64
	if err := binary.Read(crand.Reader, binary.LittleEndian, &s); err != nil {
		// crypto/rand からReadできなかった場合の代替手段
		s = time.Now().UnixNano()
	}
	rand.Seed(s)
}

func RandomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}

func UniqueRandomString(n int) string {
	s := RandomString(n)
	for {
		if _, ok := randomStringUnique.Load(s); ok {
			continue
		}
		randomStringUnique.Store(s, struct{}{})
		return s
	}
}
