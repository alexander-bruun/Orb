import { isTauri } from '$lib/utils/platform';

/**
 * Native <audio> element fallback for MP3 and 16-bit FLAC.
 * The element is created once and reused; only the src changes.
 */
export class NativePlayer {
	private el: HTMLAudioElement;
	private onEndedCb?: () => void;
	private onPositionCb?: (posMs: number) => void;
	private onDurationCb?: (durMs: number) => void;
	private onBufferedCb?: (pct: number) => void;
	private hlsRuntimeUnsupported = false;

	constructor() {
		this.el = new Audio();
		this.el.crossOrigin = 'anonymous';
		this.el.addEventListener('timeupdate', () => {
			this.onPositionCb?.(this.el.currentTime * 1000);
			this.onBufferedCb?.(this.getBufferedPct());
		});
		this.el.addEventListener('progress', () => {
			this.onBufferedCb?.(this.getBufferedPct());
		});
		this.el.addEventListener('loadedmetadata', () => {
			if (this.el.duration && isFinite(this.el.duration)) {
				this.onDurationCb?.(this.el.duration * 1000);
			}
		});
		this.el.addEventListener('ended', () => {
			this.onEndedCb?.();
		});
	}

	private getBufferedPct(): number {
		const { buffered, duration, currentTime } = this.el;
		if (!duration || !isFinite(duration)) return 0;
		for (let i = 0; i < buffered.length; i++) {
			if (buffered.start(i) <= currentTime && currentTime <= buffered.end(i)) {
				return (buffered.end(i) / duration) * 100;
			}
		}
		return 0;
	}

	private async setSourceAndPlay(src: string, startSeconds = 0): Promise<void> {
		this.el.pause();
		this.el.src = src;
		// Explicitly reset and load the new source. Without this, calling play()
		// immediately after changing src can throw NotSupportedError in Chrome
		// before the browser has determined that the media type is playable.
		this.el.load();
		await this.el.play();
		if (startSeconds > 0) {
			this.el.currentTime = startSeconds;
		}
	}

	private shouldTryHlsFirst(): boolean {
		// In Tauri (Chromium on Linux), segment requests can fail auth after the
		// first chunk, so force direct stream there.
		if (isTauri()) return false;
		// Prefer segmented HLS first for faster startup/buffering. If this browser
		// rejects HLS at runtime, we remember that and use direct stream after.
		return !this.hlsRuntimeUnsupported;
	}

	async play(url: string, token: string, startSeconds = 0): Promise<void> {
		const directSrc = `${url}?token=${encodeURIComponent(token)}`;
		const hlsSrc = `${url}/index.m3u8?token=${encodeURIComponent(token)}`;
		const preferHls = this.shouldTryHlsFirst();
		const primary = preferHls ? hlsSrc : directSrc;
		const fallback = preferHls ? directSrc : hlsSrc;
		try {
			await this.setSourceAndPlay(primary, startSeconds);
		} catch (err) {
			// Auto-fallback between direct stream and HLS manifest when the first
			// source is rejected by the browser's media pipeline.
			const msg = err instanceof Error ? err.message : String(err);
			const name = err && typeof err === 'object' && 'name' in err
				? String((err as { name?: unknown }).name)
				: '';
			const notSupported = name === 'NotSupportedError' || /supported source/i.test(msg);
			if (!notSupported) throw err;
			if (primary === hlsSrc) {
				this.hlsRuntimeUnsupported = true;
			}
			await this.setSourceAndPlay(fallback, startSeconds);
		}
	}

	pause() {
		this.el.pause();
	}

	resume() {
		this.el.play().catch(() => { });
	}

	seek(seconds: number) {
		this.el.currentTime = seconds;
	}

	setVolume(gain: number) {
		this.el.volume = Math.max(0, Math.min(1, gain));
	}

	/**
	 * Route audio output to a specific device (e.g. a Bluetooth speaker).
	 * Uses HTMLMediaElement.setSinkId() — only available in Chrome/Edge.
	 * Silently ignored on unsupported browsers.
	 */
	async setSinkId(sinkId: string): Promise<void> {
		const el = this.el as HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> };
		if (typeof el.setSinkId === 'function') {
			try {
				await el.setSinkId(sinkId);
			} catch (err) {
				console.warn('[NativePlayer] setSinkId failed:', err);
			}
		}
	}

	onEnded(cb: () => void) {
		this.onEndedCb = cb;
	}

	onPosition(cb: (posMs: number) => void) {
		this.onPositionCb = cb;
	}

	onDuration(cb: (durMs: number) => void) {
		this.onDurationCb = cb;
	}

	onBuffered(cb: (pct: number) => void) {
		this.onBufferedCb = cb;
	}

	/**
	 * Play from a blob: URL (offline downloads). Skips HLS and token logic.
	 */
	async playBlob(blobUrl: string, startSeconds = 0): Promise<void> {
		await this.setSourceAndPlay(blobUrl, startSeconds);
	}

	/** Expose the underlying HTMLAudioElement (e.g. for createMediaElementSource). */
	getElement(): HTMLAudioElement {
		return this.el;
	}

	/**
	 * True when the browser supports the Remote Playback API on this element.
	 * Available on mobile Chrome/Edge — allows casting to Chromecast, AirPlay, etc.
	 */
	get remotePlaybackSupported(): boolean {
		return 'remote' in this.el;
	}

	/**
	 * Prompt the user to select a remote playback device (Chromecast, AirPlay, etc.)
	 * using the browser's native Remote Playback API.
	 * Throws if the API is unavailable or the user cancels.
	 */
	async promptRemotePlayback(): Promise<void> {
		const el = this.el as HTMLAudioElement & { remote?: RemotePlayback };
		if (!el.remote) throw new Error('Remote Playback API not available');
		// Disable remote playback watchAvailability monitoring first, then prompt.
		await el.remote.prompt();
	}

	destroy() {
		this.el.pause();
		this.el.src = '';
		this.el.remove();
	}
}
