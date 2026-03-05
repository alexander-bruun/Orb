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

	constructor() {
		this.el = new Audio();
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

	async play(url: string, token: string, startSeconds = 0): Promise<void> {
		// Use the HLS manifest when the browser supports it natively (Safari /
		// WebKit). This gives the player segment-aware buffering and seeking.
		// Chrome and Firefox don't support HLS in <audio> natively so they fall
		// back to the direct byte-range stream which already starts quickly.
		// In Tauri (WebKitGTK on Linux), HLS is technically supported but the
		// internal segment requests don't carry the auth token, so playback
		// breaks after the first buffered chunk. Force the direct stream there.
		const canHLS =
			!isTauri() && this.el.canPlayType('application/vnd.apple.mpegurl') !== '';
		const src = canHLS
			? `${url}/index.m3u8?token=${encodeURIComponent(token)}`
			: `${url}?token=${encodeURIComponent(token)}`;
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

	pause() {
		this.el.pause();
	}

	resume() {
		this.el.play().catch(() => {});
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
		this.el.src = blobUrl;
		this.el.load();
		await this.el.play();
		if (startSeconds > 0) {
			this.el.currentTime = startSeconds;
		}
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
