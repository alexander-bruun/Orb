package ingest

import (
	"encoding/binary"
	"io"
	"os"
)

// mp3DurationMs returns the duration of an MP3 file in milliseconds.
// It reads the Xing/Info (VBR) or VBRI frame header when present, and falls
// back to a CBR estimate based on bitrate and file size.
// Returns 0 on any parse failure.
func mp3DurationMs(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0
	}
	fileSize := fi.Size()

	// Skip ID3v2 header if present.
	id3Size := skipID3v2(f)

	// Read enough bytes to find the first frame header and check for VBR tags.
	// Xing/Info: up to 156 bytes into the frame; give ourselves plenty of room.
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	buf = buf[:n]

	// Scan for the first valid MPEG frame sync.
	frameOffset := -1
	for i := 0; i < len(buf)-3; i++ {
		if buf[i] == 0xFF && buf[i+1]&0xE0 == 0xE0 {
			if parseMPEGHeader(buf[i:]) != nil {
				frameOffset = i
				break
			}
		}
	}
	if frameOffset < 0 {
		return 0
	}

	hdr := parseMPEGHeader(buf[frameOffset:])
	if hdr == nil {
		return 0
	}

	// ── VBR: Xing / Info header ───────────────────────────────────────────
	// The Xing tag sits after the side-information block inside the first frame.
	xingOffset := frameOffset + 4 + hdr.sideInfoSize
	if xingOffset+8 <= len(buf) {
		tag := string(buf[xingOffset : xingOffset+4])
		if tag == "Xing" || tag == "Info" {
			flags := binary.BigEndian.Uint32(buf[xingOffset+4:])
			if flags&0x01 != 0 && xingOffset+12 <= len(buf) {
				frames := int64(binary.BigEndian.Uint32(buf[xingOffset+8:]))
				if frames > 0 {
					return frames * int64(hdr.samplesPerFrame) * 1000 / int64(hdr.sampleRate)
				}
			}
		}
	}

	// ── VBR: VBRI header (Fraunhofer) ────────────────────────────────────
	// VBRI sits at a fixed offset of 32 bytes after the frame sync.
	vbriOffset := frameOffset + 4 + 32
	if vbriOffset+18 <= len(buf) && string(buf[vbriOffset:vbriOffset+4]) == "VBRI" {
		frames := int64(binary.BigEndian.Uint32(buf[vbriOffset+14:]))
		if frames > 0 {
			return frames * int64(hdr.samplesPerFrame) * 1000 / int64(hdr.sampleRate)
		}
	}

	// ── CBR fallback ──────────────────────────────────────────────────────
	if hdr.bitrateKbps > 0 {
		audioBytes := fileSize - id3Size
		return audioBytes * 8 / int64(hdr.bitrateKbps)
	}

	return 0
}

type mpegHeader struct {
	sampleRate      int
	bitrateKbps     int
	samplesPerFrame int
	sideInfoSize    int // bytes of side-information after the 4-byte header
}

// parseMPEGHeader parses the 4-byte MPEG frame header at the start of b.
// Returns nil if b is too short or the header is invalid.
func parseMPEGHeader(b []byte) *mpegHeader {
	if len(b) < 4 {
		return nil
	}
	h := binary.BigEndian.Uint32(b[:4])

	// Sync bits must all be set.
	if h>>21 != 0x7FF {
		return nil
	}

	versionBits := (h >> 19) & 0x3
	layerBits := (h >> 17) & 0x3
	bitrateIdx := (h >> 12) & 0xF
	srIdx := (h >> 10) & 0x3
	channelMode := (h >> 6) & 0x3

	// Only handle Layer 3.
	if layerBits != 1 {
		return nil
	}
	// Free-format and bad bitrate index.
	if bitrateIdx == 0 || bitrateIdx == 15 {
		return nil
	}
	// Reserved sample rate index.
	if srIdx == 3 {
		return nil
	}

	var mpeg1 bool
	var samplesPerFrame int
	var sampleRates [3]int
	var bitrateTable [15]int

	switch versionBits {
	case 3: // MPEG 1
		mpeg1 = true
		samplesPerFrame = 1152
		sampleRates = [3]int{44100, 48000, 32000}
		bitrateTable = [15]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	case 2: // MPEG 2
		samplesPerFrame = 576
		sampleRates = [3]int{22050, 24000, 16000}
		bitrateTable = [15]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	case 0: // MPEG 2.5
		samplesPerFrame = 576
		sampleRates = [3]int{11025, 12000, 8000}
		bitrateTable = [15]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	default:
		return nil
	}

	sampleRate := sampleRates[srIdx]
	bitrateKbps := bitrateTable[bitrateIdx]

	// Side-information size varies by version and channel mode.
	mono := channelMode == 3
	var sideInfoSize int
	if mpeg1 {
		if mono {
			sideInfoSize = 17
		} else {
			sideInfoSize = 32
		}
	} else {
		if mono {
			sideInfoSize = 9
		} else {
			sideInfoSize = 17
		}
	}

	return &mpegHeader{
		sampleRate:      sampleRate,
		bitrateKbps:     bitrateKbps,
		samplesPerFrame: samplesPerFrame,
		sideInfoSize:    sideInfoSize,
	}
}

// skipID3v2 reads past an ID3v2 header (if present) and returns the byte
// offset where the MPEG audio data begins. Leaves the file read position
// just past the ID3v2 header. Returns 0 if there is no ID3v2 header.
func skipID3v2(f *os.File) int64 {
	hdr := make([]byte, 10)
	if _, err := io.ReadFull(f, hdr); err != nil {
		_, _ = f.Seek(0, io.SeekStart)
		return 0
	}
	if string(hdr[0:3]) != "ID3" {
		_, _ = f.Seek(0, io.SeekStart)
		return 0
	}
	// ID3v2 size is a 4-byte syncsafe integer (bit 7 of each byte is 0).
	size := int64(hdr[6])<<21 | int64(hdr[7])<<14 | int64(hdr[8])<<7 | int64(hdr[9])
	// hdr[5] bit 4 = footer present (adds 10 bytes)
	if hdr[5]&0x10 != 0 {
		size += 10
	}
	total := 10 + size
	_, _ = f.Seek(total, io.SeekStart)
	return total
}
