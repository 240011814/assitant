package tts

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Edge 朗读服务 2026-10 起要求的 Sec-MS-GEC v2 鉴权 token 生成。
// token 格式 "<16hex>-<64hex>": 对 "{ticks}{TrustedClientToken}" 取变体 SHA-256
// (前 7 字节作盐派生初始状态、第 15 轮常数替换) 与 CS64 校验和。
// 算法按 edge-tts 7.3.0 (GPL-3.0) 的 drm.py 移植, 常量表与其一致。
const (
	edgeTrustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeSecMSGECVersion    = "2-143.0.3650.75"
	edgeWinEpoch           = 11644473600

	edgeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
)

var (
	edgeSHA256Initial = [8]uint32{
		0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A, 0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
	}
	edgeSHA256State = [8]uint32{
		0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A, 0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
	}
	edgeSHA256Schedule = [14]uint32{
		0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA, 0xA622EE8C, 0xD1DD9491, 0xC0DD25A0,
		0xF9E70AB6, 0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33, 0xD258B060, 0x72766564,
	}
	edgeSHA256Adjustment     uint32 = 0xC9DC06A7
	edgeSigma0Adjustment     uint32 = 0x059C0B80
	edgeSigma1Adjustment     uint32 = 0x000B92C0
	edgeSHA256RoundConstants        = [64]uint32{
		0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5, 0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
		0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3, 0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
		0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC, 0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
		0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7, 0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
		0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13, 0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
		0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3, 0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
		0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5, 0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
		0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208, 0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
	}
	edgeCS64Coefficients = [14][2]uint32{
		{0xE82E91B1, 0xB32700D0}, {0x44DE24C6, 0x88DFF51D}, {0x3BEB65BC, 0x29514038},
		{0xC89C5933, 0xB328CBDF}, {0x8D1EC211, 0xD405B4D5}, {0x314EF5D5, 0x26E55F3C},
		{0xE92491B1, 0x607801C6}, {0xF0A5B9D5, 0x82B2C1FB}, {0x06064CF3, 0xFA51A3DA},
		{0xDA79FA60, 0x207CC464}, {0xBED20560, 0x3C3C74F3}, {0x89F9C9F8, 0xD2D14B61},
		{0x35FA38C6, 0xBB04A2C1}, {0xCB49E3C6, 0xC5626167},
	}
	edgeCS64Factor      uint32 = 0x82893933
	edgeCS64Adjustment0 uint32 = 0xF2F90B1A
	edgeCS64Adjustment1 uint32 = 0x0B1A0000
)

func rotr32(v uint32, n uint) uint32 { return v>>n | v<<(32-n) }

// edgeSigma0/edgeSigma1 移植源的"截断组合旋转": 五个移位项直 XOR,
// 不补环绕移位 (v<<29 / v<<22), 与标准 SHA-256 小 sigma 不同!
func edgeSigma0(v uint32) uint32 { return v>>7 ^ v<<25 ^ v>>18 ^ v<<14 ^ v>>3 }
func edgeSigma1(v uint32) uint32 { return v>>17 ^ v<<15 ^ v>>19 ^ v<<13 ^ v>>10 }

// edgeExpandSchedule 标准 SHA-256 消息扩展: 补齐 words[firstWord:64]
func edgeExpandSchedule(words []uint32, firstWord int) {
	for i := firstWord; i < 64; i++ {
		words[i] = edgeSigma1(words[i-2]) + words[i-7] + edgeSigma0(words[i-15]) + words[i-16]
	}
}

// edgeSHA256Rounds 标准 64 轮压缩函数; k14Adjustment 为真时第 15 轮 (index 14)
// 的轮常数替换为 edgeSHA256Adjustment (变体算法要求)。可从 firstRound 中途进入。
func edgeSHA256Rounds(state *[8]uint32, words []uint32, firstRound int, k14Adjustment bool) {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	for i := firstRound; i < 64; i++ {
		k := edgeSHA256RoundConstants[i]
		if k14Adjustment && i == 14 {
			k = edgeSHA256Adjustment
		}
		s1 := rotr32(e, 6) ^ rotr32(e, 11) ^ rotr32(e, 25)
		ch := g ^ (e & (f ^ g))
		t1 := h + s1 + ch + k + words[i]
		s0 := rotr32(a, 2) ^ rotr32(a, 13) ^ rotr32(a, 22)
		maj := (a & b) | (c & (a | b))
		t2 := s0 + maj
		h, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
	}
	state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7] = a, b, c, d, e, f, g, h
}

// edgeSHA256InitState 用 7 字节盐派生变体 SHA-256 初始状态:
// 前 16 个消息字特殊构造 (14 个 0 + 盐的 BE24/BE32), 手工推到 words[30],
// 从第 15 轮 (index 14) 进入压缩函数, 结果与标准初始向量相加。
func edgeSHA256InitState(salt []byte) [8]uint32 {
	words := make([]uint32, 64)
	words[14] = uint32(salt[0])<<16 | uint32(salt[1])<<8 | uint32(salt[2])
	words[15] = binary.BigEndian.Uint32(salt[3:7])

	for i := 16; i < 30; i++ {
		v := edgeSHA256Schedule[i-16]
		if i-2 >= 14 {
			if i == 16 {
				v += edgeSigma1(words[i-2]) ^ edgeSigma1Adjustment
			} else {
				v += edgeSigma1(words[i-2])
			}
		}
		if i-7 >= 14 {
			v += words[i-7]
		}
		if i-15 >= 14 {
			if i == 29 {
				v += edgeSigma0(words[i-15]) ^ edgeSigma0Adjustment
			} else {
				v += edgeSigma0(words[i-15])
			}
		}
		words[i] = v
	}
	words[30] = edgeSigma1(words[28]) + words[23] + edgeSigma0(words[15]) + words[14] + edgeSHA256Adjustment - edgeSHA256RoundConstants[14]
	edgeExpandSchedule(words, 31)

	// 值拷贝进入压缩函数 (edgeSHA256State 是数组, 赋值即复制)
	working := edgeSHA256State
	edgeSHA256Rounds(&working, words, 14, true)

	var out [8]uint32
	for i := 0; i < 8; i++ {
		out[i] = edgeSHA256Initial[i] + working[i]
	}
	return out
}

// edgeSHA256Digest 变体 SHA-256: 输入须 >=7 字节, 前 7 字节为盐;
// 填充长度按 57+len(data) 计 (等价于数据前有 57 字节隐含前缀)。
func edgeSHA256Digest(data []byte) [32]byte {
	length := 57 + len(data)
	padLen := (55 - length) % 64
	if padLen < 0 {
		padLen += 64
	}
	padded := make([]byte, 0, len(data)-7+1+padLen+8)
	padded = append(padded, data[7:]...)
	padded = append(padded, 0x80)
	padded = append(padded, make([]byte, padLen)...)
	var lenBE [8]byte
	binary.BigEndian.PutUint64(lenBE[:], uint64(length)*8)
	padded = append(padded, lenBE[:]...)

	state := edgeSHA256InitState(data[:7])
	words := make([]uint32, 64)
	for off := 0; off < len(padded); off += 64 {
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(padded[off+i*4 : off+i*4+4])
		}
		edgeExpandSchedule(words, 16)
		working := state
		edgeSHA256Rounds(&working, words, 0, false)
		for i := 0; i < 8; i++ {
			state[i] += working[i]
		}
	}

	var out [32]byte
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:], state[i])
	}
	return out
}

func rol16(v uint32) uint32 { return v<<16 | v>>16 }

// edgeWordsLE4Even 数据按小端 u32 切词, 补零到偶数个
func edgeWordsLE4Even(data []byte) []uint32 {
	pad := (4 - len(data)%4) % 4
	padded := make([]byte, len(data)+pad)
	copy(padded, data)
	words := make([]uint32, len(padded)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(padded[i*4:])
	}
	if len(words)%2 == 1 {
		words = append(words, 0)
	}
	return words
}

// edgeCS64Digest CS64 校验和: 用 SHA-256 摘要前 8 字节派生两个奇数密钥,
// 与 14 对固定系数及数据小端词做乘法/rol16 链, 返回两个 32 位结果
func edgeCS64Digest(data []byte, digest []byte) (uint32, uint32) {
	keyA := binary.LittleEndian.Uint32(digest[0:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1
	factorA := keyA * edgeCS64Factor
	factorB := keyB * edgeCS64Factor

	terms := make([]uint32, 0, 32)
	offsets := make([]uint32, 0, 32)
	for i := range edgeCS64Coefficients {
		factor := factorB
		if i%2 == 0 {
			factor = factorA
		}
		terms = append(terms, edgeCS64Coefficients[i][0]*factor)
		offsets = append(offsets, edgeCS64Coefficients[i][1])
	}
	for i, word := range edgeWordsLE4Even(append([]byte{0}, data...)) {
		key := keyB
		rotFactor := uint32(0xC31713DB)
		if i%2 == 0 {
			key = keyA
			rotFactor = 0xEF0569FB
		}
		term := word * key
		offset := rol16(word) * rotFactor
		if i == 0 {
			term += edgeCS64Adjustment0 * factorA
			offset += edgeCS64Adjustment1
		}
		terms = append(terms, term)
		offsets = append(offsets, offset)
	}

	var swapAcc, swapLast, state, acc uint32
	for i := 0; i+1 < len(terms); i += 2 {
		x, y := terms[i], terms[i+1]
		t := rol16(offsets[i] + x)
		q := t * 0x76F50F34
		swapAcc += rol16(q)*0xC3EFEA97 + q
		t = rol16(offsets[i+1] + y)
		q = t * 0x1881CE3C
		swapLast = rol16(q)*0x35BD1EC9 + q
		swapAcc += swapLast

		state = state*keyA + x
		state = rol16(state) * 0xCF98B111
		state = rol16(state) * 0x87085B9F
		state = rol16(state) * 0x12CEB96D
		state = rol16(state) * 0x257E1D83
		acc += state
		t = state*keyB + y
		t = rol16(t) * 0xA27416F5
		t = rol16(t) * 0xD38396FF
		state = rol16(rol16(t)*0x7C932B89) * 0xBFA49F69
		acc += state
	}
	return swapLast ^ state, swapAcc ^ acc
}

// edgeClockSkew 本机与服务器的时钟差 (403 时按响应 Date 头校正后重试)
var edgeClockSkew struct {
	mu      sync.Mutex
	seconds float64
}

func edgeAdjustClockSkew(resp *http.Response) {
	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		return
	}
	serverTime, err := http.ParseTime(dateStr)
	if err != nil {
		return
	}
	edgeClockSkew.mu.Lock()
	edgeClockSkew.seconds = time.Until(serverTime).Seconds()
	edgeClockSkew.mu.Unlock()
}

// edgeGenerateSecMSGEC 生成 v2 Sec-MS-GEC token ("<16hex>-<64hex>"):
// (Unix秒+偏移+Windows纪元) 向下取整到 5 分钟 → 100ns tick → 拼 TrustedClientToken
// 后取变体 SHA-256 与 CS64。全程用 float 对齐移植源 (f"{ticks:.0f}")。
func edgeGenerateSecMSGEC() string {
	edgeClockSkew.mu.Lock()
	skew := edgeClockSkew.seconds
	edgeClockSkew.mu.Unlock()

	ticks := float64(time.Now().UnixNano())/1e9 + skew + edgeWinEpoch
	ticks -= math.Mod(ticks, 300)
	strToHash := fmt.Sprintf("%.0f%s", ticks*1e7, edgeTrustedClientToken)

	digest := edgeSHA256Digest([]byte(strToHash))
	cs64A, cs64B := edgeCS64Digest([]byte(strToHash), digest[:])
	var cs64 [8]byte
	binary.LittleEndian.PutUint32(cs64[:4], cs64A)
	binary.LittleEndian.PutUint32(cs64[4:], cs64B)

	return fmt.Sprintf("%X-%X", cs64[:], digest[:])
}

// edgeGenerateMUID 随机 MUID (Cookie 用)
func edgeGenerateMUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return strings.ToUpper(fmt.Sprintf("%X", b))
}
