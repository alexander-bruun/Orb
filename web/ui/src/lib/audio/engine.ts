/**
 * AudioEngine — unified audio interface.
 * Selects the WASM FLAC decoding path for 24-bit+ FLAC,
 * falls back to native <audio> for MP3 and 16-bit FLAC.
 *
 * Framework-agnostic: no Svelte imports.
 * The player store calls into this; components only read from the store.
 */
import { NativePlayer } from './native';
import { get } from 'svelte/store';
import { authStore } from '$lib/stores/auth';
import { positionMs, durationMs, bufferedPct, next as playerNext } from '$lib/stores/player';
import type { EQBand } from '$lib/types';

import { getApiBase } from '$lib/api/base';

class AudioEngine {
	private ctx: AudioContext | null = null;
	private gainNode: GainNode | null = null;
	/** Sits between gainNode and destination; shared by both WASM and native paths. */
	private analyserNode: AnalyserNode | null = null;
	/**
	 * Separate AudioContext used exclusively for analysing native-path audio
	 * (<audio> element). Kept separate so that native-path playback is never
	 * broken by context suspension / sample-rate switches on the WASM side.
	 */
	private nativeCtx: AudioContext | null = null;
	private nativeAnalyser: AnalyserNode | null = null;
	private nativeGain: GainNode | null = null;
	private nativeMediaSource: MediaElementSourceNode | null = null;
	private nativePlayer: NativePlayer | null = null;
	/** BiquadFilterNode chain for the WASM/Web Audio path. */
	private eqNodes: BiquadFilterNode[] = [];
	/** BiquadFilterNode chain for the native <audio> path. */
	private nativeEqNodes: BiquadFilterNode[] = [];
	/** The bands last applied via setEQ(); used to rebuild the chain after context recreation. */
	private currentEQBands: EQBand[] = [];
	/** Single-slot callback; fires whenever the full decoded AudioBuffer is ready. */
	private onBufferReadyCb: ((buf: AudioBuffer) => void) | null = null;
	private wasmActive = false;
	private loaded = false;
	// Primary source node (first segment during quick-start, or full buffer).
	private currentSource: AudioBufferSourceNode | null = null;
	// Continuation source scheduled to start after the first segment ends.
	private pendingSource: AudioBufferSourceNode | null = null;
	// Full decoded buffer — available once the background download finishes.
	private wasmFullBuffer: AudioBuffer | null = null;
	private startTime = 0;
	private offsetSeconds = 0;
	private positionInterval: ReturnType<typeof setInterval> | null = null;

	/** True after a successful play(); false after stop() or before first play. */
	get isLoaded(): boolean {
		return this.loaded;
	}

	/**
	 * Return the active AnalyserNode (for WASM / Web Audio path) or the native
	 * AnalyserNode (for the <audio> element path), or null if unavailable.
	 */
	getAnalyser(): AnalyserNode | null {
		return this.wasmActive ? this.analyserNode : this.nativeAnalyser;
	}

	/** Expose the underlying AudioContext (WASM path) so visualizers can read timing. */
	getAudioContext(): AudioContext | null {
		return this.ctx;
	}

	/**
	 * Return the fully-decoded AudioBuffer for the current track (WASM path only).
	 * Returns null on the native path or before decoding completes.
	 */
	getDecodedBuffer(): AudioBuffer | null {
		return this.wasmFullBuffer;
	}

	/**
	 * Register a one-shot callback that fires when the full decoded buffer is
	 * ready (WASM path). Replaces any previously registered callback.
	 * For the native path the callback never fires; use getDecodedBuffer() after
	 * the waveform store's fallback fetch-and-decode path.
	 */
	onBufferReady(cb: (buf: AudioBuffer) => void): void {
		this.onBufferReadyCb = cb;
	}

	/** Set the full decoded buffer, update buffered% and notify listeners. */
	private _setFullBuffer(buf: AudioBuffer): void {
		this.wasmFullBuffer = buf;
		bufferedPct.set(100);
		this.onBufferReadyCb?.(buf);
	}

	/**
	 * Return (or create) the AudioContext at exactly the requested sample rate.
	 * For 24-bit hi-res content the context MUST match the source rate so that
	 * decodeAudioData never resamples the audio. If the rate has changed the
	 * old context is closed — stop() is always called before play() so nothing
	 * is playing when this runs.
	 */
	private async getCtx(sampleRate: number): Promise<AudioContext> {
		if (this.ctx && this.ctx.sampleRate !== sampleRate) {
			this.ctx.close().catch(() => {});
			this.ctx = null;
			this.gainNode = null;
			this.analyserNode = null; // released with the context
		}
		if (!this.ctx) {
			this.ctx = new AudioContext({ sampleRate });
			this.gainNode = this.ctx.createGain();
			this.analyserNode = this.ctx.createAnalyser();
			this.analyserNode.fftSize = 2048;
			this.analyserNode.smoothingTimeConstant = 0.8;
			this.eqNodes = this._buildEQChain(this.ctx, this.gainNode, this.analyserNode, this.currentEQBands);
			this.analyserNode.connect(this.ctx.destination);
		}
		// Tauri's WebView (and some browsers) create AudioContexts in a
		// "suspended" state. Explicitly resume so playback actually produces
		// output instead of silently buffering and then cutting off.
		if (this.ctx.state === 'suspended') {
			await this.ctx.resume();
		}
		return this.ctx;
	}

	async play(trackId: string, bitDepth: number, sampleRate: number, startSeconds = 0): Promise<void> {
		this.stop();
		if (bitDepth > 16) {
			// 24-bit+ content MUST always use the Web Audio path so the full
			// dynamic range and original sample rate are preserved end-to-end.
			await this.playWasm(trackId, sampleRate, startSeconds);
		} else {
			await this.playNative(trackId, startSeconds);
		}
		this.loaded = true;
	}

	// ---------------------------------------------------------------------------
	// WASM path (24-bit+ FLAC)
	// ---------------------------------------------------------------------------

	private async playWasm(trackId: string, sampleRate: number, startSeconds: number): Promise<void> {
		const ctx = await this.getCtx(sampleRate);
		const token = get(authStore).token ?? '';
		this.wasmFullBuffer = null;

		// For non-zero start positions we need the full buffer for accurate
		// decoding, so skip the quick-start optimisation.
		if (startSeconds > 0) {
			await this.playWasmFull(trackId, token, startSeconds, ctx);
			return;
		}

		// Fetch the m3u8 manifest to learn the first segment's byte range.
		let firstSegEnd = -1;
		try {
			const mRes = await fetch(`${getApiBase()}/stream/${trackId}/index.m3u8`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (mRes.ok) {
				const text = await mRes.text();
				// Match the first EXT-X-BYTERANGE tag: LENGTH@OFFSET
				const m = text.match(/#EXT-X-BYTERANGE:(\d+)@(\d+)/);
				if (m) firstSegEnd = parseInt(m[2], 10) + parseInt(m[1], 10) - 1;
			}
		} catch {
			/* manifest unavailable — fall through to full download */
		}

		if (firstSegEnd <= 0) {
			await this.playWasmFull(trackId, token, 0, ctx);
			return;
		}

		// Race the first-segment fetch against the full-file fetch so that on a
		// fast connection or with a cached response the full buffer wins and we
		// never need the hot-swap logic.
		const segFetch = fetch(`${getApiBase()}/stream/${trackId}`, {
			headers: { Authorization: `Bearer ${token}`, Range: `bytes=0-${firstSegEnd}` }
		}).then((r) => r.arrayBuffer()).then((b) => new Uint8Array(b));

		const fullFetch = fetch(`${getApiBase()}/stream/${trackId}`, {
			headers: { Authorization: `Bearer ${token}`, Range: 'bytes=0-' }
		}).then((r) => r.arrayBuffer()).then((b) => new Uint8Array(b));

		const winner = await Promise.race([
			segFetch.then((data) => ({ type: 'seg' as const, data })),
			fullFetch.then((data) => ({ type: 'full' as const, data }))
		]);

		if (winner.type === 'full') {
			// Full file arrived before the segment — play it directly.
			const buf = await this.decodeAudioData(ctx, winner.data);
			if (!buf) {
				await this.playWasmFull(trackId, token, 0, ctx);
				return;
			}
			this._setFullBuffer(buf);
			this.startWasmPlayback(ctx, buf, 0);
			this.startPositionTracking();
			return;
		}

		// First segment won the race — decode and play it immediately.
		const segBuf = await this.decodeAudioData(ctx, winner.data);
		if (!segBuf) {
			// Partial FLAC decode failed — wait for the full file instead.
			const fullData = await fullFetch;
			await this.playWasmFull(trackId, token, 0, ctx, fullData);
			return;
		}

		const playStartTime = ctx.currentTime;
		this.startWasmPlayback(ctx, segBuf, 0);
		this.startPositionTracking();

		// Schedule the full-buffer continuation in the background. When it
		// arrives we pre-schedule it as a second AudioBufferSourceNode so the
		// Web Audio timeline handles the transition with zero gap.
		const continuationAt = playStartTime + segBuf.duration;
		fullFetch
			.then(async (fullData) => {
				if (!this.wasmActive || !this.ctx) return;
				const fullBuf = await this.decodeAudioData(ctx, fullData);
				if (!fullBuf || !this.wasmActive) return;

				durationMs.set(fullBuf.duration * 1000);
				this._setFullBuffer(fullBuf);

				// Suppress playerNext from the first-segment source — the
				// continuation source will fire it when the track truly ends.
				if (this.currentSource) {
					try { this.currentSource.onended = null; } catch { /* ignore */ }
				}

				const restSource = ctx.createBufferSource();
				restSource.buffer = fullBuf;
				restSource.connect(this.gainNode!);
				// Start at the position in the full buffer where the first
				// segment left off.
				restSource.start(continuationAt, segBuf.duration);
				restSource.onended = () => {
					if (this.pendingSource === restSource && this.wasmActive) {
						playerNext().catch(() => {});
					}
				};
				this.pendingSource = restSource;
			})
			.catch(() => {});
	}

	/** Decode a Uint8Array to an AudioBuffer, returning null on failure. */
	private async decodeAudioData(ctx: AudioContext, data: Uint8Array): Promise<AudioBuffer | null> {
		try {
			return await ctx.decodeAudioData(
				(data.buffer as ArrayBuffer).slice(data.byteOffset, data.byteOffset + data.byteLength)
			);
		} catch {
			return null;
		}
	}

	/** Create and start an AudioBufferSourceNode, replacing any current source. */
	private startWasmPlayback(ctx: AudioContext, buf: AudioBuffer, offsetSeconds: number): void {
		// Clear any existing sources.
		for (const src of [this.currentSource, this.pendingSource]) {
			if (src) {
				try { src.onended = null; } catch { /* ignore */ }
				try { src.stop(); } catch { /* ignore */ }
				try { src.disconnect(); } catch { /* ignore */ }
			}
		}
		this.pendingSource = null;

		const source = ctx.createBufferSource();
		source.buffer = buf;
		source.connect(this.gainNode!);
		source.start(0, offsetSeconds);
		source.onended = () => {
			if (this.currentSource === source && this.wasmActive) {
				playerNext().catch(() => {});
			}
		};

		this.currentSource = source;
		this.startTime = ctx.currentTime;
		this.offsetSeconds = offsetSeconds;
		durationMs.set(buf.duration * 1000);
		this.wasmActive = true;
	}

	/** Download and decode the full file then start playback. */
	private async playWasmFull(
		trackId: string,
		token: string,
		startSeconds: number,
		ctx: AudioContext,
		data?: Uint8Array
	): Promise<void> {
		if (!data) {
			const res = await fetch(`${getApiBase()}/stream/${trackId}`, {
				headers: { Authorization: `Bearer ${token}`, Range: 'bytes=0-' }
			});
			data = new Uint8Array(await res.arrayBuffer());
		}
		const buf = await ctx.decodeAudioData(
			(data.buffer as ArrayBuffer).slice(data.byteOffset, data.byteOffset + data.byteLength)
		);
		this._setFullBuffer(buf);
		this.startWasmPlayback(ctx, buf, startSeconds);
		this.startPositionTracking();
	}

	// ---------------------------------------------------------------------------
	// Native path (MP3 / 16-bit FLAC / WAV)
	// ---------------------------------------------------------------------------

	private async playNative(trackId: string, startSeconds: number): Promise<void> {
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
		const token = get(authStore).token ?? '';
		const url = `${getApiBase()}/stream/${trackId}`;
		await this.nativePlayer.play(url, token, startSeconds);

		this.nativePlayer.onPosition((ms) => positionMs.set(ms));
		this.nativePlayer.onDuration((ms) => durationMs.set(ms));
		this.nativePlayer.onBuffered((pct) => bufferedPct.set(pct));
		this.nativePlayer.onEnded(() => {
			playerNext().catch(() => {});
		});
		this.wasmActive = false;

		// Wire up the native analyser the first time (one-shot — createMediaElementSource
		// can only be called once per HTMLMediaElement).
		if (!this.nativeMediaSource) {
			try {
				const el = this.nativePlayer.getElement();
				this.nativeCtx = new AudioContext();
				this.nativeGain = this.nativeCtx.createGain();
				this.nativeAnalyser = this.nativeCtx.createAnalyser();
				this.nativeAnalyser.fftSize = 2048;
				this.nativeAnalyser.smoothingTimeConstant = 0.8;
				this.nativeEqNodes = this._buildEQChain(this.nativeCtx, this.nativeGain, this.nativeAnalyser, this.currentEQBands);
				this.nativeAnalyser.connect(this.nativeCtx.destination);
				this.nativeMediaSource = this.nativeCtx.createMediaElementSource(el);
				this.nativeMediaSource.connect(this.nativeGain);
				// Sync initial volume
				const currentGain = this.gainNode ? this.gainNode.gain.value : 1;
				this.nativeGain.gain.value = currentGain;
			} catch {
				/* analyser unavailable for native path — visualizer will show flat signal */
			}
		}
		if (this.nativeCtx?.state === 'suspended') {
			this.nativeCtx.resume().catch(() => {});
		}
	}

	// ---------------------------------------------------------------------------
	// Playback controls
	// ---------------------------------------------------------------------------

	pause() {
		if (this.wasmActive && this.ctx) {
			this.offsetSeconds += this.ctx.currentTime - this.startTime;
			this.ctx.suspend();
		} else {
			this.nativePlayer?.pause();
		}
		this.stopPositionTracking();
	}

	resume() {
		if (this.wasmActive && this.ctx) {
			this.ctx.resume().then(() => {
				this.startTime = this.ctx!.currentTime;
				this.startPositionTracking();
			});
		} else {
			this.nativePlayer?.resume();
		}
	}

	seek(positionSeconds: number) {
		if (this.wasmActive && this.ctx) {
			// Prefer the full buffer for seeking; fall back to whatever is loaded.
			const buf =
				this.wasmFullBuffer ??
				this.pendingSource?.buffer ??
				this.currentSource?.buffer ??
				null;
			if (!buf) return;

			// Stop all active/scheduled sources.
			for (const src of [this.currentSource, this.pendingSource]) {
				if (src) {
					try { src.onended = null; } catch { /* ignore */ }
					try { src.stop(); } catch { /* ignore */ }
					try { src.disconnect(); } catch { /* ignore */ }
				}
			}
			this.currentSource = null;
			this.pendingSource = null;

			const source = this.ctx.createBufferSource();
			source.buffer = buf;
			source.connect(this.gainNode!);
			source.start(0, positionSeconds);
			source.onended = () => {
				if (this.currentSource === source && this.wasmActive) {
					playerNext().catch(() => {});
				}
			};
			this.currentSource = source;
			this.offsetSeconds = positionSeconds;
			this.startTime = this.ctx.currentTime;
			this.startPositionTracking();
		} else {
			this.nativePlayer?.seek(positionSeconds);
		}
	}

	/**
	 * Apply an EQ band configuration to both audio paths.
	 * Rebuilds the BiquadFilterNode chains between the gain node and analyser node.
	 * Safe to call at any time, including during playback.
	 */
	setEQ(bands: EQBand[]): void {
		this.currentEQBands = bands;

		// Rebuild WASM path chain
		if (this.ctx && this.gainNode && this.analyserNode) {
			for (const node of this.eqNodes) {
				try { node.disconnect(); } catch { /* ignore */ }
			}
			this.eqNodes = this._buildEQChain(this.ctx, this.gainNode, this.analyserNode, bands);
		}

		// Rebuild native path chain
		if (this.nativeCtx && this.nativeGain && this.nativeAnalyser) {
			for (const node of this.nativeEqNodes) {
				try { node.disconnect(); } catch { /* ignore */ }
			}
			this.nativeEqNodes = this._buildEQChain(this.nativeCtx, this.nativeGain, this.nativeAnalyser, bands);
		}
	}

	/**
	 * Build a BiquadFilterNode chain between source and dest.
	 * Disconnects any previous direct source → dest connection and wires
	 * source → eq[0] → ... → eq[n-1] → dest.
	 * Returns the array of filter nodes (empty = flat, direct connection).
	 */
	private _buildEQChain(
		ctx: AudioContext,
		source: AudioNode,
		dest: AudioNode,
		bands: EQBand[]
	): BiquadFilterNode[] {
		// Remove the old direct connection (throws if not connected — that's fine).
		try { source.disconnect(dest); } catch { /* not directly connected */ }

		if (bands.length === 0) {
			source.connect(dest);
			return [];
		}

		const nodes = bands.map((band) => {
			const filter = ctx.createBiquadFilter();
			filter.type = band.type as BiquadFilterType;
			filter.frequency.value = band.frequency;
			filter.gain.value = band.gain;
			filter.Q.value = band.type === 'peaking' ? 1.4 : 0.7;
			return filter;
		});

		source.connect(nodes[0]);
		for (let i = 0; i < nodes.length - 1; i++) {
			nodes[i].connect(nodes[i + 1]);
		}
		nodes[nodes.length - 1].connect(dest);
		return nodes;
	}

	setVolume(gain: number) {
		const clamped = Math.max(0, Math.min(1, gain));
		if (this.gainNode) {
			this.gainNode.gain.value = clamped;
		}
		this.nativePlayer?.setVolume(clamped);
		// Keep native-path analyser gain in sync so volume changes are reflected
		// in any active visualizer.
		if (this.nativeGain) {
			this.nativeGain.gain.value = clamped;
		}
	}

	stop() {
		bufferedPct.set(0);
		this.stopPositionTracking();
		for (const src of [this.currentSource, this.pendingSource]) {
			if (src) {
				try { src.onended = null; } catch { /* ignore */ }
				try { src.stop(); } catch { /* ignore */ }
				try { src.disconnect(); } catch { /* ignore */ }
			}
		}
		this.currentSource = null;
		this.pendingSource = null;
		this.wasmFullBuffer = null;
		this.wasmActive = false;
		this.loaded = false;
		this.offsetSeconds = 0;
		if (this.nativePlayer) {
			this.nativePlayer.onEnded(() => {});
			this.nativePlayer.pause();
		}
	}

	private startPositionTracking() {
		this.stopPositionTracking();
		this.positionInterval = setInterval(() => {
			if (this.wasmActive && this.ctx) {
				const elapsed = this.ctx.currentTime - this.startTime + this.offsetSeconds;
				// Clamp to the full buffer duration so the seek bar never overshoots.
				// During the quick-start segment phase wasmFullBuffer is null, so no
				// clamp is applied until the full buffer has been decoded.
				const clampDur = this.wasmFullBuffer?.duration;
				positionMs.set((clampDur != null ? Math.min(elapsed, clampDur) : elapsed) * 1000);
			}
		}, 250);
	}

	private stopPositionTracking() {
		if (this.positionInterval) {
			clearInterval(this.positionInterval);
			this.positionInterval = null;
		}
	}
}

export const audioEngine = new AudioEngine();
