package ingest

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// m4bInfo holds the parsed duration and chapters from an M4B/M4A file.
type m4bInfo struct {
	durationMs int64
	chapters   []m4bChapter
}

type m4bChapter struct {
	startMs int64
	title   string
}

// probeM4B opens an M4B/M4A file and returns its duration and chapter list
// without requiring any external tools.
//
// Chapter sources tried in order:
//  1. Nero-style chapters in moov/udta/chpl
//  2. QuickTime chapter track (trak with "text" or "sbtl" handler)
func probeM4B(path string) (*m4bInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fi.Size()

	// Read up to 64 MB from the start of the file.
	// For files with moov at the front (streaming-optimised M4B), everything
	// — including chapter text data referenced by stco — lives here.
	headSize := fileSize
	const maxHead = 64 << 20 // 64 MB
	if headSize > maxHead {
		headSize = maxHead
	}
	head := make([]byte, headSize)
	if _, err := io.ReadFull(f, head); err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}

	info := &m4bInfo{}

	// Try moov in the first 64 MB.
	if moovBody, ok := findBox(head, "moov"); ok {
		parseMoov(moovBody, head, 0, info)
		return info, nil
	}

	// moov not found in first 64 MB — it may be at the end of the file
	// (non-optimised layout). Read the last 8 MB and try again.
	if fileSize <= maxHead {
		return info, nil // file fully read, no moov found
	}

	const tailSize = 8 << 20 // 8 MB
	tailOff := fileSize - tailSize
	tail := make([]byte, tailSize)
	if _, err2 := f.Seek(tailOff, io.SeekStart); err2 != nil {
		return info, nil
	}
	n, _ := f.Read(tail)
	tail = tail[:n]

	if moovBody, ok := findBox(tail, "moov"); ok {
		// fileBufOff = tailOff so stco absolute file offsets map correctly into tail.
		parseMoov(moovBody, tail, tailOff, info)
	}

	return info, nil
}

// parseMoov processes a moov box body.
// fileBuf is the raw file data we have buffered; fileBufOff is the absolute
// file offset of fileBuf[0]. These are forwarded to parseQTChapters so that
// stco absolute offsets can be resolved into buffer indices.
func parseMoov(moovBody []byte, fileBuf []byte, fileBufOff int64, info *m4bInfo) {
	// Total duration.
	if mvhd, ok := findBox(moovBody, "mvhd"); ok {
		parseMvhdBox(mvhd, info)
	}

	// ── Nero chapters (moov/udta/chpl) ───────────────────────────────────────
	if udta, ok := findBox(moovBody, "udta"); ok {
		if chpl, ok := findBox(udta, "chpl"); ok {
			parseChplBox(chpl, info)
		}
	}

	// ── QuickTime chapter track (fallback) ────────────────────────────────────
	if len(info.chapters) == 0 {
		parseQTChapters(moovBody, fileBuf, fileBufOff, info)
	}
}

// ── Box navigation helpers ────────────────────────────────────────────────────

// boxHeader parses the size and type of the MP4 box at buf[pos].
// Returns (headerSize, bodySize, boxType, ok).
func boxHeader(buf []byte, pos int) (headerSize, bodySize int, boxType string, ok bool) {
	if pos+8 > len(buf) {
		return 0, 0, "", false
	}
	size32 := int(binary.BigEndian.Uint32(buf[pos:]))
	boxType = string(buf[pos+4 : pos+8])

	switch {
	case size32 == 0:
		headerSize = 8
		bodySize = len(buf) - pos - 8
	case size32 == 1:
		if pos+16 > len(buf) {
			return 0, 0, "", false
		}
		total := int(binary.BigEndian.Uint64(buf[pos+8:]))
		headerSize = 16
		bodySize = total - 16
	default:
		headerSize = 8
		bodySize = size32 - 8
	}

	if bodySize < 0 || pos+headerSize+bodySize > len(buf) {
		return 0, 0, "", false
	}
	return headerSize, bodySize, boxType, true
}

// findBox returns the body of the first direct child box of the given type.
func findBox(buf []byte, typ string) ([]byte, bool) {
	pos := 0
	for {
		hs, bs, bt, ok := boxHeader(buf, pos)
		if !ok {
			break
		}
		if bt == typ {
			return buf[pos+hs : pos+hs+bs], true
		}
		if hs+bs == 0 { // size32==0 means "to end of buffer"; consumed
			break
		}
		pos += hs + bs
	}
	return nil, false
}

// findAllBoxes returns the bodies of all direct child boxes of the given type.
func findAllBoxes(buf []byte, typ string) [][]byte {
	var out [][]byte
	pos := 0
	for {
		hs, bs, bt, ok := boxHeader(buf, pos)
		if !ok {
			break
		}
		if bt == typ {
			out = append(out, buf[pos+hs:pos+hs+bs])
		}
		if hs+bs == 0 {
			break
		}
		pos += hs + bs
	}
	return out
}

// ── parseMvhdBox ─────────────────────────────────────────────────────────────

// parseMvhdBox reads the movie header box to extract total duration.
// mvhd layout (FullBox):
//
//	1 byte version + 3 bytes flags
//	version 0: creation(4) + modification(4) + timescale(4) + duration(4)
//	version 1: creation(8) + modification(8) + timescale(4) + duration(8)
func parseMvhdBox(body []byte, info *m4bInfo) {
	if len(body) < 4 {
		return
	}
	version := body[0]
	var timescale, duration int64

	switch version {
	case 0:
		if len(body) < 20 {
			return
		}
		timescale = int64(binary.BigEndian.Uint32(body[12:]))
		duration = int64(binary.BigEndian.Uint32(body[16:]))
	case 1:
		if len(body) < 32 {
			return
		}
		timescale = int64(binary.BigEndian.Uint32(body[20:]))
		duration = int64(binary.BigEndian.Uint64(body[24:]))
	default:
		return
	}

	if timescale > 0 {
		info.durationMs = duration * 1000 / timescale
	}
}

// ── parseChplBox ─────────────────────────────────────────────────────────────

// parseChplBox reads Nero-style chapter data from the chpl box.
// chpl layout (FullBox):
//
//	1 byte version + 3 bytes flags
//	version 1: 4 bytes unknown/reserved
//	4 bytes chapter count
//	per chapter: 8 bytes start (100ns units) + 1 byte title len + N bytes title
func parseChplBox(body []byte, info *m4bInfo) {
	if len(body) < 8 {
		return
	}
	version := body[0]
	off := 4 // skip version + flags

	if version == 1 {
		off += 4 // skip reserved
	}
	if off+4 > len(body) {
		return
	}

	count := int(binary.BigEndian.Uint32(body[off:]))
	off += 4

	info.chapters = make([]m4bChapter, 0, count)
	for i := 0; i < count; i++ {
		if off+9 > len(body) {
			break
		}
		startNs100 := binary.BigEndian.Uint64(body[off:])
		titleLen := int(body[off+8])
		off += 9
		if off+titleLen > len(body) {
			break
		}
		title := string(body[off : off+titleLen])
		off += titleLen

		info.chapters = append(info.chapters, m4bChapter{
			startMs: int64(startNs100) / 10_000,
			title:   title,
		})
	}
}

// ── parseQTChapters ──────────────────────────────────────────────────────────

// parseQTChapters looks for QuickTime-style chapter tracks inside moovBody.
//
// Structure:
//
//	moov/trak           (one per track; we look for handler "text"/"sbtl")
//	  mdia/hdlr         → 4-byte handler type at offset 8
//	  mdia/mdhd         → timescale for this track
//	  mdia/minf/stbl
//	    stts            → (sample_count, sample_delta) pairs — chapter durations
//	    stco / co64     → absolute file offsets of each text sample
//
// Text sample layout: 2-byte big-endian length + UTF-8 title.
//
// fileBuf[0] corresponds to absolute file offset fileBufOff, so the buffer
// index for a stco offset O is (O − fileBufOff).
func parseQTChapters(moovBody []byte, fileBuf []byte, fileBufOff int64, info *m4bInfo) {
	for _, trak := range findAllBoxes(moovBody, "trak") {
		mdia, ok := findBox(trak, "mdia")
		if !ok {
			continue
		}

		// ── Handler type ──────────────────────────────────────────────────
		// hdlr FullBox: 4 version+flags | 4 pre_defined | 4 handler_type | …
		hdlr, ok := findBox(mdia, "hdlr")
		if !ok || len(hdlr) < 12 {
			continue
		}
		handlerType := string(hdlr[8:12])
		if handlerType != "text" && handlerType != "sbtl" {
			continue
		}

		// ── Timescale from mdhd ───────────────────────────────────────────
		// v0: 4 version+flags | 4 creation | 4 modification | 4 timescale | …
		// v1: 4 version+flags | 8 creation | 8 modification | 4 timescale | …
		mdhd, ok := findBox(mdia, "mdhd")
		if !ok || len(mdhd) < 4 {
			continue
		}
		var timescale int64
		switch mdhd[0] {
		case 0:
			if len(mdhd) >= 16 {
				timescale = int64(binary.BigEndian.Uint32(mdhd[12:]))
			}
		case 1:
			if len(mdhd) >= 28 {
				timescale = int64(binary.BigEndian.Uint32(mdhd[24:]))
			}
		}
		if timescale <= 0 {
			timescale = 1000
		}

		// ── Navigate to stbl ──────────────────────────────────────────────
		minf, ok := findBox(mdia, "minf")
		if !ok {
			continue
		}
		stbl, ok := findBox(minf, "stbl")
		if !ok {
			continue
		}

		// ── stts: sample timing ───────────────────────────────────────────
		// FullBox: 4 version+flags | 4 entry_count | N × (4 count, 4 delta)
		sttsBody, ok := findBox(stbl, "stts")
		if !ok || len(sttsBody) < 8 {
			continue
		}
		sttsCount := int(binary.BigEndian.Uint32(sttsBody[4:]))
		if len(sttsBody) < 8+sttsCount*8 {
			continue
		}
		type sttsEntry struct{ count, delta uint32 }
		stts := make([]sttsEntry, sttsCount)
		for i := range stts {
			stts[i].count = binary.BigEndian.Uint32(sttsBody[8+i*8:])
			stts[i].delta = binary.BigEndian.Uint32(sttsBody[12+i*8:])
		}
		if len(stts) == 0 {
			continue
		}

		// ── stco / co64: absolute chunk file offsets ──────────────────────
		var chunkOffsets []int64
		if b, ok := findBox(stbl, "stco"); ok && len(b) >= 8 {
			n := int(binary.BigEndian.Uint32(b[4:]))
			if len(b) >= 8+n*4 {
				chunkOffsets = make([]int64, n)
				for i := range chunkOffsets {
					chunkOffsets[i] = int64(binary.BigEndian.Uint32(b[8+i*4:]))
				}
			}
		} else if b, ok := findBox(stbl, "co64"); ok && len(b) >= 8 {
			n := int(binary.BigEndian.Uint32(b[4:]))
			if len(b) >= 8+n*8 {
				chunkOffsets = make([]int64, n)
				for i := range chunkOffsets {
					chunkOffsets[i] = int64(binary.BigEndian.Uint64(b[8+i*8:]))
				}
			}
		}

		// ── Build chapters ────────────────────────────────────────────────
		// Chapter text tracks have 1 sample per chunk in practice.
		// Cumulative stts deltas give chapter start times; stco gives where
		// the 2-byte-prefixed UTF-8 title is stored in the file.
		var chapters []m4bChapter
		var cumulativeMs int64
		chunkIdx := 0

		for _, e := range stts {
			for j := uint32(0); j < e.count; j++ {
				startMs := cumulativeMs
				cumulativeMs += int64(e.delta) * 1000 / timescale

				var title string
				if chunkIdx < len(chunkOffsets) {
					bufIdx := chunkOffsets[chunkIdx] - fileBufOff
					if bufIdx >= 0 && bufIdx+2 <= int64(len(fileBuf)) {
						textLen := int(binary.BigEndian.Uint16(fileBuf[bufIdx:]))
						end := bufIdx + 2 + int64(textLen)
						if textLen > 0 && end <= int64(len(fileBuf)) {
							title = string(fileBuf[bufIdx+2 : end])
						}
					}
				}
				if title == "" {
					title = fmt.Sprintf("Chapter %d", chunkIdx+1)
				}

				chapters = append(chapters, m4bChapter{
					startMs: startMs,
					title:   title,
				})
				chunkIdx++
			}
		}

		if len(chapters) > 0 {
			info.chapters = chapters
			return
		}
	}
}
