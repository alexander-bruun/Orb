/**
 * waveformPeaks store
 *
 * Computes and caches a flat array of amplitude peaks for the currently-playing
 * track so the TrackWaveform component can render an Audacity-style overview.
 *
 * Strategy:
 *  1. When currentTrack changes, immediately try audioEngine.getDecodedBuffer()
 *     (available on the 24-bit WASM path as soon as the full file is decoded).
 *  2. Register audioEngine.onBufferReady() so we learn as soon as the WASM
 *     buffer becomes available (races the quick-start segment phase).
 *  3. After 800 ms, if still waiting, assume native path (<audio>) and fetch
 *     the raw audio ourselves, decode it with a temporary AudioContext, then
 *     compute peaks from that.
 *
 * The peaks array is normalised to [0, 1] with 1000 bars. This resolution
 * is more than enough for a 300 px canvas and can be simply sub-sampled for
 * narrower displays.
 */
import { writable, get } from 'svelte/store';
import { currentTrack } from '$lib/stores/player';
import { audioEngine } from '$lib/audio/engine';
import { authStore } from '$lib/stores/auth';
import { getApiBase } from '$lib/api/base';

export interface WaveformPeaks {
	trackId: string;
	/** Normalised peak amplitudes, length = NUM_BARS, values in [0, 1]. */
	peaks: Float32Array;
}

export const waveformPeaks = writable<WaveformPeaks | null>(null);
export const waveformLoading = writable(false);

const NUM_BARS = 1000;

/**
 * Downsample an AudioBuffer to NUM_BARS peak values, yielding control every
 * 50 bars so the main thread doesn't stutter on hi-res multiminute tracks.
 */
async function computePeaks(buf: AudioBuffer): Promise<Float32Array> {
	const numCh = buf.numberOfChannels;
	const total = buf.length;
	const samplesPerBar = Math.ceil(total / NUM_BARS);

	// Pull channel data upfront (cheap typed-array views).
	const channels: Float32Array[] = [];
	for (let c = 0; c < numCh; c++) channels.push(buf.getChannelData(c));

	const peaks = new Float32Array(NUM_BARS);
	let globalMax = 0;

	for (let i = 0; i < NUM_BARS; i++) {
		// Yield every 50 bars to stay within ~16 ms frame budget per batch.
		if (i % 50 === 0 && i > 0) {
			await new Promise<void>((r) => setTimeout(r, 0));
		}

		const start = i * samplesPerBar;
		const end = Math.min(start + samplesPerBar, total);
		let maxAbs = 0;

		for (let s = start; s < end; s++) {
			// Mix channels to mono.
			let mono = 0;
			for (let c = 0; c < numCh; c++) mono += channels[c][s];
			mono /= numCh;
			const abs = mono < 0 ? -mono : mono;
			if (abs > maxAbs) maxAbs = abs;
		}

		peaks[i] = maxAbs;
		if (maxAbs > globalMax) globalMax = maxAbs;
	}

	// Normalise to [0, 1] so quiet tracks still fill the canvas.
	if (globalMax > 0) {
		for (let i = 0; i < NUM_BARS; i++) peaks[i] /= globalMax;
	}

	return peaks;
}

// ── Subscription ─────────────────────────────────────────────────────────────

let generation = 0; // incremented on each track change to discard stale work

currentTrack.subscribe(async (track) => {
	const gen = ++generation;
	waveformPeaks.set(null);

	if (!track) {
		waveformLoading.set(false);
		return;
	}

	waveformLoading.set(true);
	let applied = false;

	/** Apply peaks for the current generation (drops stale results). */
	async function applyBuf(buf: AudioBuffer) {
		if (gen !== generation) return;
		const peaks = await computePeaks(buf);
		if (gen !== generation) return;
		applied = true;
		waveformPeaks.set({ trackId: track!.id, peaks });
		waveformLoading.set(false);
	}

	// Path 1: WASM buffer already decoded (e.g. track resumed after pause,
	// or store subscribes after audio has started playing).
	const existing = audioEngine.getDecodedBuffer();
	if (existing) {
		await applyBuf(existing);
		return;
	}

	// Path 2: WASM buffer will become available soon — register a callback.
	audioEngine.onBufferReady((buf) => {
		if (gen !== generation || applied) return;
		applyBuf(buf);
	});

	// Path 3: If no WASM buffer after 800 ms, we're on the native path.
	// Fetch the full audio and decode it ourselves just for the waveform.
	await new Promise<void>((r) => setTimeout(r, 800));
	if (gen !== generation || applied) return;

	try {
		const token = get(authStore).token ?? '';
		const res = await fetch(`${getApiBase()}/stream/${track.id}`, {
			headers: { Authorization: `Bearer ${token}`, Range: 'bytes=0-' }
		});
		if (gen !== generation) return;

		const data = await res.arrayBuffer();
		if (gen !== generation) return;

		// Decode in a temporary AudioContext (not used for playback).
		const tmpCtx = new AudioContext();
		let decoded: AudioBuffer;
		try {
			decoded = await tmpCtx.decodeAudioData(data);
		} finally {
			tmpCtx.close().catch(() => {});
		}
		if (gen !== generation) return;

		await applyBuf(decoded);
	} catch {
		if (gen === generation) waveformLoading.set(false);
	}
});
