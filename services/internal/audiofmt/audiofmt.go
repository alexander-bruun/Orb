// Package audiofmt implements the audio format selection engine for multi-channel playback.
//
// The backend is capability-aware: given a device's audio capabilities and a
// track's available audio formats, SelectFormat returns the best AudioFormat
// to stream, following the priority chain:
//
//	Atmos passthrough → 7.1 PCM → 5.1 PCM → stereo fallback
package audiofmt

import (
	"github.com/alexander-bruun/orb/services/internal/store"
)

// FormatResult is the output of SelectFormat — the chosen stream URL and
// codec metadata to return to the client.
type FormatResult struct {
	// FileKey is the object-store key for the selected audio file.
	// Empty string means use the track's primary file_key (stereo).
	FileKey     string `json:"file_key"`
	Codec       string `json:"codec"`
	Channels    int    `json:"channels"`
	Passthrough bool   `json:"passthrough"`
	// LayoutType is the selected layout name: "stereo", "5.1", "7.1", "atmos"
	LayoutType string `json:"layout_type"`
}

// SelectFormat returns the best audio format for the given device capabilities
// and track. The fallback chain is: Atmos → 7.1 → 5.1 → stereo.
//
// If the track has no AudioFormats populated (i.e. it was ingested before
// multi-channel support), a stereo result pointing at the track's primary
// file is returned.
func SelectFormat(caps store.AudioCapabilities, track store.Track) FormatResult {
	// Atmos passthrough — requires explicit device support.
	if caps.SupportsPassthrough && track.HasAtmos {
		if f := findFormat(track.AudioFormats, "atmos"); f != nil && codecAllowed(caps, f.Codec) {
			return FormatResult{
				FileKey:     f.FileKey,
				Codec:       f.Codec,
				Channels:    f.Channels,
				Passthrough: true,
				LayoutType:  "atmos",
			}
		}
	}

	// 7.1 PCM
	if caps.MaxChannels >= 8 {
		if f := findFormat(track.AudioFormats, "7.1"); f != nil {
			return FormatResult{
				FileKey:    f.FileKey,
				Codec:      f.Codec,
				Channels:   f.Channels,
				LayoutType: "7.1",
			}
		}
	}

	// 5.1 PCM
	if caps.MaxChannels >= 6 {
		if f := findFormat(track.AudioFormats, "5.1"); f != nil {
			return FormatResult{
				FileKey:    f.FileKey,
				Codec:      f.Codec,
				Channels:   f.Channels,
				LayoutType: "5.1",
			}
		}
	}

	// Stereo fallback — use explicit stereo format entry if present,
	// otherwise fall back to the track's primary file_key.
	if f := findFormat(track.AudioFormats, "stereo"); f != nil {
		return FormatResult{
			FileKey:    f.FileKey,
			Codec:      f.Codec,
			Channels:   f.Channels,
			LayoutType: "stereo",
		}
	}

	// No AudioFormats at all — use primary file (pre-multi-channel track).
	codec := track.Format
	channels := track.Channels
	if channels == 0 {
		channels = 2
	}
	return FormatResult{
		FileKey:    track.FileKey,
		Codec:      codec,
		Channels:   channels,
		LayoutType: "stereo",
	}
}

// findFormat returns the first AudioFormat matching the given layout type, or nil.
func findFormat(formats []store.AudioFormat, layoutType string) *store.AudioFormat {
	for i := range formats {
		if formats[i].Type == layoutType {
			return &formats[i]
		}
	}
	return nil
}

// codecAllowed returns true if the codec is in the device's passthrough codec
// allowlist, or if no allowlist is set (meaning all codecs are permitted).
func codecAllowed(caps store.AudioCapabilities, codec string) bool {
	if len(caps.PassthroughCodecs) == 0 {
		return true
	}
	for _, c := range caps.PassthroughCodecs {
		if c == codec {
			return true
		}
	}
	return false
}
