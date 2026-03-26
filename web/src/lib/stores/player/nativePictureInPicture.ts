import { get } from 'svelte/store';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { currentTrack, positionMs, durationMs } from './musicPlayer';
import { currentAudiobook, abPositionMs, abDurationMs } from './audiobookPlayer';
import { activePlayer } from './engine';
import { lyricsLines, activeLyricIndex } from './lyrics';
import { waveformPeaks } from './waveformPeaks';

let bridgeVideo: HTMLVideoElement | null = null;
let bridgeCanvas: HTMLCanvasElement | null = null;
let bridgeDrawTimer: ReturnType<typeof setInterval> | null = null;
let bridgeSourceKind: 'wasm' | 'native' | null = null;
let coverImage: HTMLImageElement | null = null;
let coverImageKey = '';

function supportsNativePiP(): boolean {
	if (typeof window === 'undefined' || typeof document === 'undefined') return false;
	return 'pictureInPictureEnabled' in document;
}

function currentSourceKind(): 'wasm' | 'native' | null {
	if (!audioEngine.isLoaded) return null;
	return audioEngine.isWasmActive ? 'wasm' : 'native';
}

function ensureBridgeCanvas(): HTMLCanvasElement {
	if (bridgeCanvas) return bridgeCanvas;
	bridgeCanvas = document.createElement('canvas');
	bridgeCanvas.width = 512;
	bridgeCanvas.height = 512;
	return bridgeCanvas;
}

function currentCoverKey(): string {
	const mode = get(activePlayer);
	if (mode === 'audiobook') {
		const book = get(currentAudiobook);
		return book?.id ? `audiobook:${book.id}` : '';
	}
	const track = get(currentTrack);
	return track?.album_id ? `album:${track.album_id}` : '';
}

function currentCoverUrl(): string {
	const mode = get(activePlayer);
	const base = getApiBase();
	if (mode === 'audiobook') {
		const book = get(currentAudiobook);
		return book?.id ? `${base}/covers/audiobook/${book.id}` : '';
	}
	const track = get(currentTrack);
	return track?.album_id ? `${base}/covers/${track.album_id}` : '';
}

function ensureCoverImage(): HTMLImageElement | null {
	const key = currentCoverKey();
	if (!key) {
		coverImage = null;
		coverImageKey = '';
		return null;
	}
	if (coverImage && coverImageKey === key) return coverImage;

	const url = currentCoverUrl();
	const img = new Image();
	img.crossOrigin = 'anonymous';
	img.decoding = 'async';
	img.src = url;
	coverImage = img;
	coverImageKey = key;
	return coverImage;
}

function drawBridgeFrame(): void {
	const canvas = ensureBridgeCanvas();
	const ctx = canvas.getContext('2d');
	if (!ctx) return;

	const mode = get(activePlayer);
	const track = get(currentTrack);
	const book = get(currentAudiobook);
	const title = mode === 'audiobook' ? (book?.title ?? 'Audiobook') : (track?.title ?? 'Now Playing');
	const subtitle = mode === 'audiobook'
		? (book?.author_name ?? '')
		: (track?.artist_name ?? '');
	const lyricIdx = mode === 'music' ? get(activeLyricIndex) : -1;
	const lines = mode === 'music' ? get(lyricsLines) : [];
	const lyricLine = lyricIdx >= 0 ? (lines[lyricIdx]?.text ?? '') : '';
	const posMs = mode === 'audiobook' ? get(abPositionMs) : get(positionMs);
	const durMs = mode === 'audiobook' ? get(abDurationMs) : get(durationMs);
	const progress = durMs > 0 ? Math.max(0, Math.min(1, posMs / durMs)) : 0;

	const art = ensureCoverImage();
	ctx.fillStyle = '#111827';
	ctx.fillRect(0, 0, canvas.width, canvas.height);
	if (art && art.complete && art.naturalWidth > 0) {
		try {
			const iw = art.naturalWidth;
			const ih = art.naturalHeight;
			const scale = Math.max(canvas.width / iw, canvas.height / ih);
			const dw = iw * scale;
			const dh = ih * scale;
			const dx = (canvas.width - dw) / 2;
			const dy = (canvas.height - dh) / 2;
			ctx.drawImage(art, dx, dy, dw, dh);
		} catch {
			// Keep fallback block if drawing fails.
		}
	}

	const overlay = ctx.createLinearGradient(0, 0, 0, 200);
	overlay.addColorStop(0, 'rgba(0, 0, 0, 0.72)');
	overlay.addColorStop(1, 'rgba(0, 0, 0, 0)');
	ctx.fillStyle = overlay;
	ctx.fillRect(0, 0, canvas.width, 200);

	ctx.fillStyle = '#ffffff';
	ctx.font = '700 30px system-ui, -apple-system, Segoe UI, Roboto, sans-serif';
	ctx.fillText(title.slice(0, 24), 24, 52);

	ctx.fillStyle = 'rgba(232, 238, 251, 0.95)';
	ctx.font = '500 22px system-ui, -apple-system, Segoe UI, Roboto, sans-serif';
	if (subtitle) ctx.fillText(subtitle.slice(0, 34), 24, 86);

	if (lyricLine) {
		ctx.fillStyle = 'rgba(0, 0, 0, 0.42)';
		ctx.fillRect(20, 98, canvas.width - 40, 36);
		ctx.fillStyle = 'rgba(245, 250, 255, 0.96)';
		ctx.font = '500 18px system-ui, -apple-system, Segoe UI, Roboto, sans-serif';
		ctx.fillText(lyricLine.slice(0, 44), 30, 122);
	}

	const barX = 24;
	const barY = canvas.height - 52;
	const barW = canvas.width - 48;
	const barH = 24;
	const barMid = barY + barH / 2;
	const cursorX = barX + barW * progress;
	const trackId = track?.id ?? '';
	const wf = get(waveformPeaks);
	const peaks = mode === 'music' && wf?.trackId === trackId ? wf.peaks : null;

	ctx.fillStyle = 'rgba(0, 0, 0, 0.30)';
	ctx.fillRect(barX, barY, barW, barH);

	if (peaks && peaks.length > 0) {
		for (let px = 0; px < barW; px++) {
			const t = px / barW;
			const idx = Math.min(Math.floor(t * peaks.length), peaks.length - 1);
			const peak = Math.max(0, Math.min(1, peaks[idx]));
			const amp = Math.max(1, peak * (barH / 2));
			ctx.fillStyle = (barX + px) < cursorX ? '#7db1ff' : 'rgba(220, 233, 255, 0.42)';
			ctx.fillRect(barX + px, barMid - amp, 1, amp * 2);
		}
		// Playhead cursor
		ctx.fillStyle = 'rgba(255, 255, 255, 0.95)';
		ctx.fillRect(Math.max(barX, Math.min(barX + barW - 1, cursorX)), barY - 1, 1, barH + 2);
	} else {
		// Fallback while peaks are not ready.
		ctx.fillStyle = 'rgba(255, 255, 255, 0.35)';
		ctx.fillRect(barX, barMid - 2, barW, 4);
		ctx.fillStyle = '#7db1ff';
		ctx.fillRect(barX, barMid - 2, Math.max(0, Math.min(barW, barW * progress)), 4);
	}
}

function ensureDrawTimer(): void {
	drawBridgeFrame();
	if (bridgeDrawTimer) return;
	bridgeDrawTimer = setInterval(drawBridgeFrame, 120);
}

function stopDrawTimer(): void {
	if (!bridgeDrawTimer) return;
	clearInterval(bridgeDrawTimer);
	bridgeDrawTimer = null;
}

function buildBridgeStream(): MediaStream | null {
	const audioStream = audioEngine.getPiPAudioStream();
	if (!audioStream) return null;

	const canvas = ensureBridgeCanvas();
	const videoTrack = canvas.captureStream(12).getVideoTracks()[0];
	if (!videoTrack) return null;

	const out = new MediaStream();
	out.addTrack(videoTrack);
	for (const track of audioStream.getAudioTracks()) out.addTrack(track);
	return out;
}

async function ensureBridgeVideo(): Promise<HTMLVideoElement | null> {
	const stream = buildBridgeStream();
	if (!stream) return null;

	if (!bridgeVideo) {
		const v = document.createElement('video');
		v.autoplay = true;
		v.playsInline = true;
		v.muted = true;
		v.volume = 0;
		v.controls = false;
		v.style.position = 'fixed';
		v.style.right = '0';
		v.style.bottom = '0';
		v.style.width = '1px';
		v.style.height = '1px';
		v.style.opacity = '0';
		v.style.pointerEvents = 'none';
		v.setAttribute('aria-hidden', 'true');
		document.body.appendChild(v);
		bridgeVideo = v;
	}

	const sourceKind = currentSourceKind();
	if (bridgeVideo.srcObject !== stream || bridgeSourceKind !== sourceKind) {
		bridgeVideo.srcObject = stream;
		bridgeSourceKind = sourceKind;
	}
	return bridgeVideo;
}

export async function syncNativePictureInPictureBridge(playing: boolean): Promise<void> {
	if (!supportsNativePiP()) return;
	if (!audioEngine.isLoaded) {
		await teardownNativePictureInPictureBridge();
		return;
	}

	ensureDrawTimer();
	const video = await ensureBridgeVideo();
	if (!video) return;

	// Keep the bridge video playing even while audio is paused so Chrome PiP
	// retains the rendered frame/text instead of showing a blank window.
	// The `playing` flag is still used by callers for state semantics; teardown
	// on true stop/idle is handled separately via audioEngine.isLoaded checks.
	await video.play().catch(() => {});
}

export async function openNativePictureInPicture(): Promise<void> {
	if (!supportsNativePiP()) throw new Error('Native Picture-in-Picture is not supported');
	const playing = true;
	await syncNativePictureInPictureBridge(playing);
	if (!bridgeVideo) throw new Error('No bridge video available for Picture-in-Picture');
	if (document.pictureInPictureElement === bridgeVideo) return;
	await bridgeVideo.requestPictureInPicture();
}

export async function closeNativePictureInPicture(): Promise<void> {
	if (typeof document === 'undefined') return;
	if (!document.pictureInPictureElement) return;
	await document.exitPictureInPicture().catch(() => {});
}

export async function teardownNativePictureInPictureBridge(): Promise<void> {
	stopDrawTimer();
	if (!bridgeVideo) return;
	if (typeof document !== 'undefined' && document.pictureInPictureElement === bridgeVideo) {
		await document.exitPictureInPicture().catch(() => {});
	}
	try {
		bridgeVideo.pause();
		bridgeVideo.srcObject = null;
		bridgeVideo.remove();
	} catch {
		// no-op
	}
	bridgeVideo = null;
	bridgeSourceKind = null;
}
