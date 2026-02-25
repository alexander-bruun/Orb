import { apiStream } from '$lib/api/client';

const CHUNK_SIZE = 256 * 1024; // 256KB

export interface Chunk {
	data: Uint8Array;
	offset: number;
}

/**
 * Streamer manages HTTP range request fetching and a ring buffer of raw chunks.
 * Callers call `read(offset, length)` and get back Uint8Array data.
 */
export class Streamer {
	private trackId: string;
	private fileSize = 0;
	private prefetchOffset = 0;
	private cache = new Map<number, Uint8Array>();
	private inflight = new Set<number>();

	constructor(trackId: string) {
		this.trackId = trackId;
	}

	async init(): Promise<number> {
		// Head request to get file size.
		const res = await apiStream(`/stream/${this.trackId}`, 0, CHUNK_SIZE - 1);
		const cr = res.headers.get('Content-Range');
		if (cr) {
			const m = cr.match(/bytes \d+-\d+\/(\d+)/);
			if (m) this.fileSize = parseInt(m[1], 10);
		}
		const data = new Uint8Array(await res.arrayBuffer());
		this.cache.set(0, data);
		return this.fileSize;
	}

	async read(offset: number, length: number): Promise<Uint8Array> {
		const chunkStart = Math.floor(offset / CHUNK_SIZE) * CHUNK_SIZE;
		if (!this.cache.has(chunkStart)) {
			await this.fetchChunk(chunkStart);
		}
		const chunk = this.cache.get(chunkStart)!;
		const chunkOffset = offset - chunkStart;
		const available = Math.min(length, chunk.length - chunkOffset);
		const result = chunk.slice(chunkOffset, chunkOffset + available);

		// Pre-fetch the next chunk.
		const nextChunk = chunkStart + CHUNK_SIZE;
		if (nextChunk < this.fileSize && !this.cache.has(nextChunk) && !this.inflight.has(nextChunk)) {
			this.fetchChunk(nextChunk).catch(() => {});
		}
		return result;
	}

	private async fetchChunk(offset: number): Promise<void> {
		if (this.inflight.has(offset)) return;
		this.inflight.add(offset);
		try {
			const end = Math.min(offset + CHUNK_SIZE - 1, this.fileSize - 1);
			const res = await apiStream(`/stream/${this.trackId}`, offset, end);
			const data = new Uint8Array(await res.arrayBuffer());
			this.cache.set(offset, data);
			// Evict old chunks to keep memory bounded (~8MB = 32 chunks max).
			if (this.cache.size > 32) {
				const oldest = this.cache.keys().next().value;
				if (oldest !== undefined) this.cache.delete(oldest);
			}
		} finally {
			this.inflight.delete(offset);
		}
	}

	getBitDepth(res: Response): number {
		return parseInt(res.headers.get('X-Orb-Bit-Depth') ?? '16', 10);
	}

	getSampleRate(res: Response): number {
		return parseInt(res.headers.get('X-Orb-Sample-Rate') ?? '44100', 10);
	}

	get size(): number {
		return this.fileSize;
	}

	destroy() {
		this.cache.clear();
		this.inflight.clear();
	}
}
