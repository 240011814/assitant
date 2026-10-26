package api

import "testing"

func TestAudioContentType(t *testing.T) {
	wav := append([]byte("RIFF"), []byte("....WAVEfmt ")...)
	if got := audioContentType(wav); got != "audio/wav" {
		t.Errorf("WAV content-type = %s, want audio/wav", got)
	}
	mp3 := append([]byte("ID3"), []byte("\x04\x00\x00\x00")...)
	if got := audioContentType(mp3); got != "audio/mpeg" {
		t.Errorf("MP3 content-type = %s, want audio/mpeg", got)
	}
}
