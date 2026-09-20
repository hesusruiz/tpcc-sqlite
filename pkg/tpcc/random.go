package tpcc

import (
	"math/rand"
	"strings"
	"sync"
	"time"
)

var syllables = [...]string{
	"BAR", "OUGHT", "ABLE", "PRI", "PRES",
	"ESE", "ANTI", "CALLY", "ATION", "EING",
}

// NURandConstants stores the C values for TPC-C non-uniform random distribution
type NURandConstants struct {
	CLast int
	CID   int
	COl   int
}

var (
	nuRandOnce  sync.Once
	globalNURand NURandConstants
)

// InitNURand initializes the non-uniform random constants C_LAST, C_ID, C_OL_I_ID
func InitNURand() {
	nuRandOnce.Do(func() {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		globalNURand = NURandConstants{
			CLast: r.Intn(256),
			CID:   r.Intn(1024),
			COl:   r.Intn(8192),
		}
	})
}

// RandGen wraps a per-goroutine math/rand.Rand for high-concurrency lock-free random generation
type RandGen struct {
	r *rand.Rand
}

// NewRandGen creates a new independent random generator
func NewRandGen() *RandGen {
	InitNURand()
	return &RandGen{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// IntRange returns a random integer in [min, max] inclusive
func (rg *RandGen) IntRange(min, max int) int {
	if min >= max {
		return min
	}
	return min + rg.r.Intn(max-min+1)
}

// FloatRange returns a random float64 in [min, max] rounded to specified decimal places
func (rg *RandGen) FloatRange(min, max float64, decimals int) float64 {
	val := min + rg.r.Float64()*(max-min)
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10.0
	}
	return float64(int64(val*pow+0.5)) / pow
}

// NURand generates non-uniform random integer in [x, y]
// Formula: (((Random(0, A) | Random(x, y)) + C) % (y - x + 1)) + x
func (rg *RandGen) NURand(a, x, y int, c int) int {
	return (((rg.IntRange(0, a) | rg.IntRange(x, y)) + c) % (y - x + 1)) + x
}

// NURandCID generates customer ID using NURand
func (rg *RandGen) NURandCID(maxCID int) int {
	return rg.NURand(1023, 1, maxCID, globalNURand.CID)
}

// NURandItemID generates item ID using NURand
func (rg *RandGen) NURandItemID(maxItem int) int {
	return rg.NURand(8191, 1, maxItem, globalNURand.COl)
}

// NURandLastName generates customer last name using NURand
func (rg *RandGen) NURandLastName() string {
	num := rg.NURand(255, 0, 999, globalNURand.CLast)
	return MakeLastName(num)
}

// MakeLastName creates a last name from a number 0..999 using the 10 syllables
func MakeLastName(num int) string {
	var sb strings.Builder
	sb.WriteString(syllables[(num/100)%10])
	sb.WriteString(syllables[(num/10)%10])
	sb.WriteString(syllables[num%10])
	return sb.String()
}

const alphaNumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const numeric = "0123456789"

// AString generates a random alphanumeric string of random length in [minLen, maxLen]
func (rg *RandGen) AString(minLen, maxLen int) string {
	length := rg.IntRange(minLen, maxLen)
	b := make([]byte, length)
	for i := range b {
		b[i] = alphaNumeric[rg.r.Intn(len(alphaNumeric))]
	}
	return string(b)
}

// NString generates a random numeric string of random length in [minLen, maxLen]
func (rg *RandGen) NString(minLen, maxLen int) string {
	length := rg.IntRange(minLen, maxLen)
	b := make([]byte, length)
	for i := range b {
		b[i] = numeric[rg.r.Intn(len(numeric))]
	}
	return string(b)
}

// ZipString generates a random ZIP code: 4 numeric chars + "11111"
func (rg *RandGen) ZipString() string {
	return rg.NString(4, 4) + "11111"
}

// StateString generates a 2-character uppercase state code
func (rg *RandGen) StateString() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string([]byte{letters[rg.r.Intn(len(letters))], letters[rg.r.Intn(len(letters))]})
}

// DataWithOriginal generates string where 10% of strings contain "ORIGINAL"
func (rg *RandGen) DataWithOriginal(minLen, maxLen int) string {
	s := rg.AString(minLen, maxLen)
	if rg.IntRange(1, 10) == 1 && len(s) >= 8 {
		pos := rg.IntRange(0, len(s)-8)
		s = s[:pos] + "ORIGINAL" + s[pos+8:]
	}
	return s
}
