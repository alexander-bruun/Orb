import { apiFetch } from './client';
import type { EQBand, EQProfile, GenreEQMapping } from '$lib/types';

const BASE = '/user/eq-profiles';

// ──── Profile CRUD ────────────────────────────────────────────

export async function listProfiles(): Promise<EQProfile[]> {
	return apiFetch<EQProfile[]>(BASE);
}

export async function getProfile(id: string): Promise<EQProfile> {
	return apiFetch<EQProfile>(`${BASE}/${id}`);
}

export async function createProfile(data: {
	name: string;
	bands: EQBand[];
	is_default?: boolean;
}): Promise<EQProfile> {
	return apiFetch<EQProfile>(BASE, {
		method: 'POST',
		body: JSON.stringify({ name: data.name, bands: data.bands, is_default: data.is_default ?? false })
	});
}

export async function updateProfile(id: string, data: { name: string; bands: EQBand[] }): Promise<EQProfile> {
	return apiFetch<EQProfile>(`${BASE}/${id}`, {
		method: 'PUT',
		body: JSON.stringify(data)
	});
}

export async function deleteProfile(id: string): Promise<void> {
	return apiFetch<void>(`${BASE}/${id}`, { method: 'DELETE' });
}

export async function setDefaultProfile(id: string): Promise<void> {
	return apiFetch<void>(`${BASE}/${id}/default`, { method: 'POST' });
}

// ──── Genre mappings ──────────────────────────────────────────

export async function listGenreMappings(): Promise<GenreEQMapping[]> {
	return apiFetch<GenreEQMapping[]>('/user/genre-eq');
}

export async function setGenreMapping(genreId: string, profileId: string): Promise<void> {
	return apiFetch<void>(`/user/genre-eq/${genreId}`, {
		method: 'PUT',
		body: JSON.stringify({ profile_id: profileId })
	});
}

export async function deleteGenreMapping(genreId: string): Promise<void> {
	return apiFetch<void>(`/user/genre-eq/${genreId}`, { method: 'DELETE' });
}

// ──── Import / Export ─────────────────────────────────────────

export function exportProfile(profile: EQProfile): void {
	const data = JSON.stringify({ name: profile.name, bands: profile.bands }, null, 2);
	const blob = new Blob([data], { type: 'application/json' });
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = `${profile.name.replace(/\s+/g, '_')}_eq.json`;
	a.click();
	URL.revokeObjectURL(url);
}

export function importProfileFromFile(file: File): Promise<{ name: string; bands: EQBand[] }> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = (e) => {
			try {
				const parsed = JSON.parse(e.target?.result as string);
				if (!parsed.name || !Array.isArray(parsed.bands)) {
					reject(new Error('Invalid EQ profile file'));
					return;
				}
				resolve({ name: parsed.name, bands: parsed.bands });
			} catch {
				reject(new Error('Failed to parse file'));
			}
		};
		reader.onerror = () => reject(new Error('Failed to read file'));
		reader.readAsText(file);
	});
}
