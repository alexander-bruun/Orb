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
import { getOfflineBlob, getOfflineBlobUrl } from '$lib/stores/offline/downloads';

import { getApiBase } from '$lib/api/base';

class AudioEngine {
	private ctx: AudioContext | null = null;
	private gainNode: GainNode | null = null;
	/** Dedicated gain node for ReplayGain offset. Sits between gainNode and the EQ chain. */
	private replayGainNode: GainNode | null = null;
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
	/** Dedicated gain node for ReplayGain offset on the native path. */
	private nativeReplayGainNode: GainNode | null = null;
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
	/** One-shot callback for crossfade scheduling; fires (and clears) when full buffer is ready. */
	private onFullBufferCrossfadeCb: ((buf: AudioBuffer) => void) | null = null;
	private wasmActive = false;
	private loaded = false;
	/**
	 * Silent looping <audio> element used as an iOS AudioSession wake-lock.
	 * When the WASM Web Audio path is active, iOS may not show lock-screen
	 * media controls unless a native media element is also playing.
	 * This silent element bridges that gap at effectively zero volume.
	 */
	private iosWakeLockEl: HTMLAudioElement | null = null;
	/** Last known linear volume (0.0–1.0). Applied to nodes when they are created. */
	private currentVolume = 1;
	// Primary source node (first segment during quick-start, or full buffer).
	private currentSource: AudioBufferSourceNode | null = null;
	// Continuation source scheduled to start after the first segment ends.
	private pendingSource: AudioBufferSourceNode | null = null;
	// Full decoded buffer — available once the background download finishes.
	private wasmFullBuffer: AudioBuffer | null = null;
	private startTime = 0;
	private offsetSeconds = 0;
	private positionInterval: ReturnType<typeof setInterval> | null = null;
	/** Blob URL for offline playback — must be revoked when done. */
	private offlineBlobUrl: string | null = null;

	// ── Crossfade / gapless ────────────────────────────────────────────────────
	/**
	 * Per-track gain node inserted between each WASM source and gainNode.
	 * Allows fading the current track out independently of the main volume.
	 * Connects: source → crossfadeGain → gainNode
	 */
	private crossfadeGain: GainNode | null = null;
	/** Preloaded AudioBuffer for the upcoming track (populated by preloadNext()). */
	private nextBuffer: AudioBuffer | null = null;
	/** Track ID of the last successfully preloaded buffer (for cache reuse). */
	private nextBufferTrackId: string | null = null;
	/** setTimeout handle for the crossfade; cleared on stop/seek. */
	private crossfadeTimer: ReturnType<typeof setTimeout> | null = null;
	/** UI callback fired when the next track begins playing (at crossfade start). */
	private onCrossfadeTransition: (() => void) | null = null;
	/** Source node for the outgoing track during crossfade; stopped after fade completes. */
	private outgoingSource: AudioBufferSourceNode | null = null;
	/** Gain node for the outgoing track during crossfade; disconnected after fade completes. */
	private outgoingGain: GainNode | null = null;

	/** True after a successful play(); false after stop() or before first play. */
	get isLoaded(): boolean {
		return this.loaded;
	}

	/** True when the WASM (Web Audio) path is active for the current track. */
	get isWasmActive(): boolean {
		return this.wasmActive;
	}

	/** True when a next-track buffer has been preloaded and is ready to crossfade. */
	get hasPreloadedNext(): boolean {
		return this.nextBuffer !== null;
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

	/**
	 * Register a one-shot callback for crossfade scheduling that fires when the
	 * full decoded buffer is available. If the buffer is already ready it fires
	 * immediately. Replaces any previous crossfade-ready callback.
	 */
	onFullBufferForCrossfade(cb: (buf: AudioBuffer) => void): void {
		if (this.wasmFullBuffer) {
			cb(this.wasmFullBuffer);
		} else {
			this.onFullBufferCrossfadeCb = cb;
		}
	}

	/**
	 * Prime the audio pipeline synchronously within a user gesture so that
	 * AudioContext creation/resume and the <audio> element unlock happen in the
	 * correct gesture window — before any async awaits that would break the
	 * browser's user-activation tracking.
	 *
	 * Call this as the very first synchronous step inside any async click handler
	 * that precedes a network fetch before playback (e.g. Start Radio).
	 */
	prime(sampleRate = 44100): void {
		// Web Audio path: create the context now so it exists within the gesture.
		if (!this.ctx || this.ctx.sampleRate !== sampleRate) {
			try {
				if (this.ctx) this.ctx.close().catch(() => {});
				this.ctx = new AudioContext({ sampleRate });
				this.gainNode = this.ctx.createGain();
				this.gainNode.gain.value = this.currentVolume;
				this.replayGainNode = this.ctx.createGain();
				this.gainNode.connect(this.replayGainNode);
				this.analyserNode = this.ctx.createAnalyser();
				this.analyserNode.fftSize = 2048;
				this.analyserNode.smoothingTimeConstant = 0.8;
				this.eqNodes = this._buildEQChain(this.ctx, this.replayGainNode, this.analyserNode, this.currentEQBands);
				this.analyserNode.connect(this.ctx.destination);
			} catch {
				/* ignore — will retry in getCtx() */
			}
		}
		if (this.ctx?.state === 'suspended') {
			this.ctx.resume().catch(() => {});
		}
		// Native <audio> path: unlock the element within the gesture by
		// instantiating the player (which creates the <audio> element) now so the
		// subsequent async el.play() call after the network fetch is allowed.
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
	}

	/** Set the full decoded buffer, update buffered% and notify listeners. */
	private _setFullBuffer(buf: AudioBuffer): void {
		this.wasmFullBuffer = buf;
		bufferedPct.set(100);
		this.onBufferReadyCb?.(buf);
		// Fire and clear the crossfade-ready callback if registered.
		if (this.onFullBufferCrossfadeCb) {
			const cb = this.onFullBufferCrossfadeCb;
			this.onFullBufferCrossfadeCb = null;
			cb(buf);
		}
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
			this.replayGainNode = null;
			this.analyserNode = null; // released with the context
		}
		if (!this.ctx) {
			this.ctx = new AudioContext({ sampleRate });
			this.gainNode = this.ctx.createGain();
			this.gainNode.gain.value = this.currentVolume;
			this.replayGainNode = this.ctx.createGain();
			this.gainNode.connect(this.replayGainNode);
			this.analyserNode = this.ctx.createAnalyser();
			this.analyserNode.fftSize = 2048;
			this.analyserNode.smoothingTimeConstant = 0.8;
			this.eqNodes = this._buildEQChain(this.ctx, this.replayGainNode, this.analyserNode, this.currentEQBands);
			this.analyserNode.connect(this.ctx.destination);
		}
		// Some desktop WebViews (Tauri) and certain browsers create AudioContexts
		// in a "suspended" state. Explicitly resume so playback actually produces
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
		this.wasmFullBuffer = null;

		// Offline: decode directly from IndexedDB blob (works on insecure origins)
		try {
			const offlineBlob = await getOfflineBlob(trackId);
			if (offlineBlob) {
				const data = new Uint8Array(await offlineBlob.arrayBuffer());
				const buf = await ctx.decodeAudioData(
					(data.buffer as ArrayBuffer).slice(data.byteOffset, data.byteOffset + data.byteLength)
				);
				this._setFullBuffer(buf);
				this.startWasmPlayback(ctx, buf, startSeconds);
				this.startPositionTracking();
				return;
			}
		} catch { /* IDB unavailable — fall through to network */ }

		const token = get(authStore).token ?? '';

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
				// Connect through the shared crossfadeGain so crossfade gain
				// ramps affect both the segment and the continuation uniformly.
				restSource.connect(this.crossfadeGain ?? this.gainNode!);
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
		// Cancel any in-flight crossfade from a previous track.
		this._cancelCrossfade();

		// Clear any existing sources.
		for (const src of [this.currentSource, this.pendingSource]) {
			if (src) {
				try { src.onended = null; } catch { /* ignore */ }
				try { src.stop(); } catch { /* ignore */ }
				try { src.disconnect(); } catch { /* ignore */ }
			}
		}
		this.pendingSource = null;

		// Disconnect the old crossfadeGain if present.
		if (this.crossfadeGain) {
			try { this.crossfadeGain.disconnect(); } catch { /* ignore */ }
			this.crossfadeGain = null;
		}

		// Insert a per-track gain node for crossfade control (transparent at 1.0).
		const cfGain = ctx.createGain();
		cfGain.gain.value = 1;
		cfGain.connect(this.gainNode!);
		this.crossfadeGain = cfGain;

		const source = ctx.createBufferSource();
		source.buffer = buf;
		source.connect(cfGain);
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
		this.startIosWakeLock();
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
	// Crossfade / gapless (WASM path only)
	// ---------------------------------------------------------------------------

	/**
	 * Preload the next track's audio into memory so a crossfade or gapless
	 * transition can be scheduled. WASM/24-bit path only; no-op otherwise.
	 * Best-effort: errors are swallowed so playback is never blocked.
	 * Only preloads when the next track's sample rate matches the current context.
	 */
	async preloadNext(trackId: string, sampleRate: number): Promise<void> {
		// Reuse the existing preloaded buffer if it's already for this track.
		if (this.nextBuffer && this.nextBufferTrackId === trackId) return;
		this.nextBuffer = null;
		this.nextBufferTrackId = null;
		if (!this.wasmActive || !this.ctx) return;
		// If sample rates differ we'd need to resample — skip for lossless accuracy.
		if (this.ctx.sampleRate !== sampleRate) return;

		const token = get(authStore).token ?? '';
		try {
			const offlineBlob = await getOfflineBlob(trackId);
			let data: Uint8Array;
			if (offlineBlob) {
				data = new Uint8Array(await offlineBlob.arrayBuffer());
			} else {
				const res = await fetch(`${getApiBase()}/stream/${trackId}`, {
					headers: { Authorization: `Bearer ${token}`, Range: 'bytes=0-' }
				});
				if (!res.ok) return;
				data = new Uint8Array(await res.arrayBuffer());
			}
			const buf = await this.decodeAudioData(this.ctx, data);
			if (buf) {
				this.nextBuffer = buf;
				this.nextBufferTrackId = trackId;
			}
		} catch { /* best-effort */ }
	}

	/**
	 * Schedule a crossfade (or gapless) transition to the preloaded next buffer.
	 * `crossfadeSecs = 0` → gapless: next source starts sample-accurately at
	 * the exact moment the current track ends, with no gain ramping.
	 * `crossfadeSecs > 0` → crossfade: overlapping gain ramps over that window.
	 *
	 * `onTransition` is called at the moment the next track starts playing so
	 * the player store can update currentTrack, queue index, etc.
	 *
	 * No-op when: no next buffer preloaded, not on WASM path, or too little
	 * time remaining in the current track to schedule anything.
	 */
	scheduleCrossfade(crossfadeSecs: number, onTransition: () => void): void {
		this._cancelCrossfade();
		if (!this.ctx || !this.wasmActive || !this.nextBuffer) return;

		const ctx = this.ctx;
		const buf = this.wasmFullBuffer ?? this.currentSource?.buffer;
		if (!buf) return;

		const effectiveDuration = buf.duration - this.offsetSeconds;
		const trackEnd = this.startTime + effectiveDuration;
		// When to start playing the next track.
		const crossfadeAt = trackEnd - crossfadeSecs;
		const delayMs = (crossfadeAt - ctx.currentTime) * 1000;

		// Need at least 200 ms of runway to do anything meaningful.
		if (delayMs < 200) return;

		const nextBuf = this.nextBuffer;
		this.onCrossfadeTransition = onTransition;

		// --- Web Audio scheduling (sample-accurate) ---
		const nextGain = ctx.createGain();
		nextGain.connect(this.gainNode!);

		const nextSource = ctx.createBufferSource();
		nextSource.buffer = nextBuf;
		nextSource.connect(nextGain);

		if (crossfadeSecs > 0) {
			// Fade current track out over [crossfadeAt, crossfadeAt + crossfadeSecs].
			const cf = this.crossfadeGain;
			if (cf) {
				cf.gain.setValueAtTime(1, crossfadeAt);
				cf.gain.linearRampToValueAtTime(0, crossfadeAt + crossfadeSecs);
			}
			// Fade next track in over the same window.
			nextGain.gain.setValueAtTime(0, crossfadeAt);
			nextGain.gain.linearRampToValueAtTime(1, crossfadeAt + crossfadeSecs);
		} else {
			// Gapless: next track plays at full volume from the start.
			nextGain.gain.value = 1;
		}

		// Schedule the next source to start at the crossfade point.
		nextSource.start(crossfadeAt);
		nextSource.onended = () => {
			if (this.currentSource === nextSource && this.wasmActive) {
				playerNext().catch(() => {});
			}
		};

		// Suppress the outgoing sources so they don't double-trigger playerNext.
		const suppressEnded = (src: AudioBufferSourceNode | null) => {
			if (src) try { src.onended = null; } catch { /* ignore */ }
		};

		// --- JS timer to update state when the transition fires ---
		this.crossfadeTimer = setTimeout(() => {
			this.crossfadeTimer = null;

			suppressEnded(this.currentSource);
			suppressEnded(this.pendingSource);

			// Save outgoing references for cleanup after the fade completes.
			this.outgoingSource = this.pendingSource ?? this.currentSource;
			this.outgoingGain = this.crossfadeGain;

			// Promote the next track as the active one.
			this.currentSource = nextSource;
			this.pendingSource = null;
			this.crossfadeGain = nextGain;
			this.startTime = crossfadeAt;
			this.offsetSeconds = 0;
			this.wasmFullBuffer = nextBuf;
			this.nextBuffer = null;
			durationMs.set(nextBuf.duration * 1000);

			// Notify player store to update UI (currentTrack, queueIndex, etc.).
			const cb = this.onCrossfadeTransition;
			this.onCrossfadeTransition = null;
			cb?.();

			// After the fade window, stop and disconnect the outgoing source.
			const cleanupDelay = Math.max(0, crossfadeSecs) * 1000 + 100;
			setTimeout(() => {
				if (this.outgoingSource) {
					try { this.outgoingSource.stop(); } catch { /* ignore */ }
					try { this.outgoingSource.disconnect(); } catch { /* ignore */ }
					this.outgoingSource = null;
				}
				if (this.outgoingGain) {
					try { this.outgoingGain.disconnect(); } catch { /* ignore */ }
					this.outgoingGain = null;
				}
			}, cleanupDelay);
		}, delayMs);
	}

	/** Cancel any pending crossfade timer and clean up outgoing nodes. */
	private _cancelCrossfade(): void {
		if (this.crossfadeTimer !== null) {
			clearTimeout(this.crossfadeTimer);
			this.crossfadeTimer = null;
		}
		this.onCrossfadeTransition = null;
		this.onFullBufferCrossfadeCb = null;
		if (this.outgoingSource) {
			try { this.outgoingSource.stop(); } catch { /* ignore */ }
			try { this.outgoingSource.disconnect(); } catch { /* ignore */ }
			this.outgoingSource = null;
		}
		if (this.outgoingGain) {
			try { this.outgoingGain.disconnect(); } catch { /* ignore */ }
			this.outgoingGain = null;
		}
	}

	// ---------------------------------------------------------------------------
	// Native path (MP3 / 16-bit FLAC / WAV)
	// ---------------------------------------------------------------------------

	private async playNative(trackId: string, startSeconds: number): Promise<void> {
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}

		// Offline: play from IndexedDB blob URL (works on insecure origins)
		let usedOffline = false;
		try {
			const blobUrl = await getOfflineBlobUrl(trackId);
			if (blobUrl) {
				this.offlineBlobUrl = blobUrl;
				await this.nativePlayer.playBlob(blobUrl, startSeconds);
				usedOffline = true;
			}
		} catch { /* IDB unavailable — fall through to network */ }

		if (!usedOffline) {
			const token = get(authStore).token ?? '';
			const url = `${getApiBase()}/stream/${trackId}`;
			await this.nativePlayer.play(url, token, startSeconds);
		}

		this.nativePlayer.onPosition((ms) => positionMs.set(ms));
		this.nativePlayer.onDuration((ms) => durationMs.set(ms));
		this.nativePlayer.onBuffered((pct) => bufferedPct.set(pct));
		this.nativePlayer.onEnded(() => {
			playerNext().catch(() => {});
		});
		this.wasmActive = false;
		this.stopIosWakeLock();

		// Wire up the native analyser the first time (one-shot — createMediaElementSource
		// can only be called once per HTMLMediaElement).
		if (!this.nativeMediaSource) {
			try {
				const el = this.nativePlayer.getElement();
				this.nativeCtx = new AudioContext();
				// Android Chrome auto-suspends AudioContexts when the tab is backgrounded.
				// Because audio from the <audio> element is routed through this context via
				// createMediaElementSource, a suspended context silences playback entirely
				// and removes the media notification from the shade. Auto-resume immediately
				// whenever a suspension occurs while a track is loaded.
				this.nativeCtx.addEventListener('statechange', () => {
					if (this.nativeCtx?.state === 'suspended' && this.loaded && !this.wasmActive) {
						this.nativeCtx.resume().catch(() => {});
					}
				});
				this.nativeGain = this.nativeCtx.createGain();
				this.nativeReplayGainNode = this.nativeCtx.createGain();
				this.nativeGain.connect(this.nativeReplayGainNode);
				this.nativeAnalyser = this.nativeCtx.createAnalyser();
				this.nativeAnalyser.fftSize = 2048;
				this.nativeAnalyser.smoothingTimeConstant = 0.8;
				this.nativeEqNodes = this._buildEQChain(this.nativeCtx, this.nativeReplayGainNode, this.nativeAnalyser, this.currentEQBands);
				this.nativeAnalyser.connect(this.nativeCtx.destination);
				this.nativeMediaSource = this.nativeCtx.createMediaElementSource(el);
				this.nativeMediaSource.connect(this.nativeGain);
				// Sync initial volume from engine's stored value so restores apply
				// even when the WASM gain node hasn't been created yet.
				this.nativeGain.gain.value = this.currentVolume;
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

	/**
	 * Resume any suspended AudioContexts. Called when the page regains
	 * visibility (e.g. user returns to Chrome from another app on Android).
	 */
	resumeAllContexts() {
		if (this.ctx?.state === 'suspended') this.ctx.resume().catch(() => {});
		if (this.nativeCtx?.state === 'suspended') this.nativeCtx.resume().catch(() => {});
	}

	seek(positionSeconds: number) {
		if (this.wasmActive && this.ctx) {
			// Cancel any in-flight crossfade — seek invalidates the scheduled timing.
			// Keep nextBuffer: the preloaded data is still valid for a re-schedule.
			this._cancelCrossfade();

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

			// Disconnect and recreate the crossfade gain so seek starts clean.
			if (this.crossfadeGain) {
				try { this.crossfadeGain.disconnect(); } catch { /* ignore */ }
				this.crossfadeGain = null;
			}
			const cfGain = this.ctx.createGain();
			cfGain.gain.value = 1;
			cfGain.connect(this.gainNode!);
			this.crossfadeGain = cfGain;

			const source = this.ctx.createBufferSource();
			source.buffer = buf;
			source.connect(cfGain);
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
	 * Rebuilds the BiquadFilterNode chains between the replay-gain node and analyser node.
	 * Safe to call at any time, including during playback.
	 */
	setEQ(bands: EQBand[]): void {
		this.currentEQBands = bands;

		// Rebuild WASM path chain
		if (this.ctx && this.replayGainNode && this.analyserNode) {
			for (const node of this.eqNodes) {
				try { node.disconnect(); } catch { /* ignore */ }
			}
			this.eqNodes = this._buildEQChain(this.ctx, this.replayGainNode, this.analyserNode, bands);
		}

		// Rebuild native path chain
		if (this.nativeCtx && this.nativeReplayGainNode && this.nativeAnalyser) {
			for (const node of this.nativeEqNodes) {
				try { node.disconnect(); } catch { /* ignore */ }
			}
			this.nativeEqNodes = this._buildEQChain(this.nativeCtx, this.nativeReplayGainNode, this.nativeAnalyser, bands);
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
		this.currentVolume = clamped;
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

	/**
	 * Apply a ReplayGain offset (in dB) to the dedicated replay-gain nodes on
	 * both audio paths. Pass 0 to disable (unity gain).
	 * The conversion is: linear = 10^(dB/20).
	 */
	setReplayGainDb(db: number): void {
		const linear = db === 0 ? 1 : Math.pow(10, db / 20);
		if (this.replayGainNode) {
			this.replayGainNode.gain.value = linear;
		}
		if (this.nativeReplayGainNode) {
			this.nativeReplayGainNode.gain.value = linear;
		}
	}

	/**
	 * Route audio output to a specific device (e.g. a Bluetooth speaker or HDMI
	 * output).  Delegates to the native <audio> element via setSinkId() for the
	 * native path.  The WASM/Web Audio path routes through AudioContext whose
	 * destination is always the default system output — no setSinkId equivalent
	 * exists for AudioContext, so only tracks on the native path are affected.
	 */
	/**
	 * Return the underlying HTMLAudioElement used by the native path,
	 * lazily creating it if needed. Used by the Remote Playback API for
	 * mobile casting.
	 */
	getMediaElement(): HTMLAudioElement {
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
		return this.nativePlayer.getElement();
	}

	/**
	 * True when the browser supports the Remote Playback API on the
	 * native <audio> element. Available on mobile Chrome/Edge.
	 */
	get remotePlaybackSupported(): boolean {
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
		return this.nativePlayer.remotePlaybackSupported;
	}

	/**
	 * Prompt the user to select a remote playback device via the
	 * browser's native Remote Playback API (Chromecast, AirPlay, etc.).
	 */
	async promptRemotePlayback(): Promise<void> {
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
		await this.nativePlayer.promptRemotePlayback();
	}

	async setAudioOutput(sinkId: string): Promise<void> {
		// Ensure the NativePlayer exists (it may not have been created yet).
		if (!this.nativePlayer) {
			this.nativePlayer = new NativePlayer();
		}
		await this.nativePlayer.setSinkId(sinkId);
		// Also apply to the iOS wake-lock element if it exists.
		if (this.iosWakeLockEl) {
			const el = this.iosWakeLockEl as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
			if (typeof el.setSinkId === 'function') {
				el.setSinkId(sinkId).catch(() => {});
			}
		}
	}

	stop() {
		bufferedPct.set(0);
		this.stopPositionTracking();
		this.stopIosWakeLock();
		this._cancelCrossfade();
		this.nextBuffer = null;
		this.nextBufferTrackId = null;
		for (const src of [this.currentSource, this.pendingSource]) {
			if (src) {
				try { src.onended = null; } catch { /* ignore */ }
				try { src.stop(); } catch { /* ignore */ }
				try { src.disconnect(); } catch { /* ignore */ }
			}
		}
		this.currentSource = null;
		this.pendingSource = null;
		if (this.crossfadeGain) {
			try { this.crossfadeGain.disconnect(); } catch { /* ignore */ }
			this.crossfadeGain = null;
		}
		this.wasmFullBuffer = null;
		this.wasmActive = false;
		this.loaded = false;
		this.offsetSeconds = 0;
		if (this.nativePlayer) {
			this.nativePlayer.onEnded(() => {});
			this.nativePlayer.pause();
		}
		// Free any outstanding offline blob URL to avoid memory leaks.
		if (this.offlineBlobUrl) {
			URL.revokeObjectURL(this.offlineBlobUrl);
			this.offlineBlobUrl = null;
		}
	}

	/**
	 * Start a silent looping audio element so iOS keeps the AudioSession alive
	 * while the WASM Web Audio path is playing.  On iOS 15.3 and earlier the
	 * Media Session lock-screen controls only appear when a native media element
	 * is active; this element acts as that bridge at effectively zero volume.
	 */
	private startIosWakeLock() {
		if (this.iosWakeLockEl) return; // already running
		try {
			// Build a minimal 1-frame silent WAV dynamically — no network request needed.
			const sampleRate = 8000;
			const numSamples = 800; // 100 ms
			const buf = new ArrayBuffer(44 + numSamples * 2);
			const v = new DataView(buf);
			const s = (o: number, str: string) => { for (let i = 0; i < str.length; i++) v.setUint8(o + i, str.charCodeAt(i)); };
			s(0, 'RIFF'); v.setUint32(4, 36 + numSamples * 2, true);
			s(8, 'WAVE'); s(12, 'fmt '); v.setUint32(16, 16, true);
			v.setUint16(20, 1, true); v.setUint16(22, 1, true);
			v.setUint32(24, sampleRate, true); v.setUint32(28, sampleRate * 2, true);
			v.setUint16(32, 2, true); v.setUint16(34, 16, true);
			s(36, 'data'); v.setUint32(40, numSamples * 2, true);
			const bytes = new Uint8Array(buf);
			let b64 = '';
			for (let i = 0; i < bytes.length; i++) b64 += String.fromCharCode(bytes[i]);
			const src = 'data:audio/wav;base64,' + btoa(b64);

			const el = document.createElement('audio');
			el.src = src;
			el.loop = true;
			el.volume = 0;
			el.play().catch(() => {});
			this.iosWakeLockEl = el;
		} catch {
			// Non-browser env (SSR) or policy block — ignore.
		}
	}

	private stopIosWakeLock() {
		if (!this.iosWakeLockEl) return;
		try { this.iosWakeLockEl.pause(); } catch { /* ignore */ }
		this.iosWakeLockEl.src = '';
		this.iosWakeLockEl = null;
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
