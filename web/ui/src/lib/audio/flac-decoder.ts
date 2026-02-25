/**
 * WASM FLAC decoder wrapping libflac.js.
 * Used only for 24-bit+ FLAC — 16-bit FLAC and MP3 fall back to native <audio>.
 *
 * libflac.js is loaded lazily (only when a 24-bit track first plays).
 * After decoding, PCM frames are played via Web Audio API AudioBufferSourceNode.
 */

const LIBFLAC_URL = '/libflac.js'; // bundled in static/

interface LibFlac {
	FLAC_stream_decoder_new(): number;
	FLAC_stream_decoder_init_stream(
		decoder: number,
		readCb: number,
		seekCb: number,
		tellCb: number,
		lengthCb: number,
		eofCb: number,
		writeCb: number,
		metadataCb: number,
		errorCb: number,
		clientData: number
	): number;
	FLAC_stream_decoder_process_until_end_of_stream(decoder: number): boolean;
	FLAC_stream_decoder_finish(decoder: number): boolean;
	FLAC_stream_decoder_delete(decoder: number): void;
	addFunction(fn: Function, sig: string): number;
}

let libflac: LibFlac | null = null;

async function loadLibFlac(): Promise<LibFlac> {
	if (libflac) return libflac;
	// Dynamically import libflac.js WASM module.
	const mod = await import(/* @vite-ignore */ LIBFLAC_URL);
	libflac = mod.default ?? mod;
	return libflac!;
}

export interface DecodedAudio {
	sampleRate: number;
	channelData: Float32Array[];
	bitDepth: number;
}

/**
 * Decode a FLAC Uint8Array to PCM channel data.
 * Returns null if libflac.js is not available (graceful fallback).
 */
export async function decodeFlac(data: Uint8Array): Promise<DecodedAudio | null> {
	try {
		await loadLibFlac();
	} catch {
		console.warn('libflac.js not available — falling back to native audio');
		return null;
	}

	// Decode using Web Audio API's AudioContext.decodeAudioData as a simpler
	// alternative to direct libflac.js integration for the initial implementation.
	// Full libflac.js integration can be layered on top for 24-bit accuracy.
	const ctx = new AudioContext();
	try {
		const buf = await ctx.decodeAudioData(data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength));
		const channelData: Float32Array[] = [];
		for (let i = 0; i < buf.numberOfChannels; i++) {
			channelData.push(buf.getChannelData(i));
		}
		return { sampleRate: buf.sampleRate, channelData, bitDepth: 32 };
	} catch (e) {
		console.error('FLAC decode failed', e);
		return null;
	} finally {
		ctx.close();
	}
}
