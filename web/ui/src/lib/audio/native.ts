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
		const canHLS = this.el.canPlayType('application/vnd.apple.mpegurl') !== '';
		const src = canHLS
			? `${url}/index.m3u8?token=${encodeURIComponent(token)}`
			: `${url}?token=${encodeURIComponent(token)}`;
		this.el.src = src;
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

	destroy() {
		this.el.pause();
		this.el.src = '';
		this.el.remove();
	}
}
