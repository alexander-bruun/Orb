/**
 * eq store — manages EQ profiles and genre mappings.
 *
 * Usage:
 *   import { eqProfiles, activeEQProfile, loadEQProfiles, applyEQProfile, disableEQ } from '$lib/stores/eq';
 */
import { writable, derived, get } from 'svelte/store';
import type { EQBand, EQProfile, GenreEQMapping } from '$lib/types';
import { DEFAULT_EQ_BANDS } from '$lib/types';
import * as eqApi from '$lib/api/eq';
import { audioEngine } from '$lib/audio/engine';

// ── Native Android EQ sync ────────────────────────────────────────────────────

function isAndroidNative(): boolean {
	if (typeof window === 'undefined') return false;
	return (window as unknown as { __TAURI_METADATA__?: { currentPlatform?: string } }).__TAURI_METADATA__?.currentPlatform === 'android';
}

async function nativeSyncEQ(enabled: boolean, bands: EQBand[]): Promise<void> {
	if (!isAndroidNative()) return;
	try {
		const { invoke } = await import('@tauri-apps/api/core');
		await invoke('set_eq_bands', {
			enabled,
			bandsJson: enabled ? JSON.stringify(bands) : '[]'
		});
	} catch { /* best-effort */ }
}

// ──────────────────────────────────────────────────────────────
// State stores
// ──────────────────────────────────────────────────────────────

/** All EQ profiles owned by the current user. */
export const eqProfiles = writable<EQProfile[]>([]);

/** The profile whose bands are currently loaded into the audio engine, or null for flat. */
export const activeEQProfile = writable<EQProfile | null>(null);

/** All genre → profile mappings for the current user. */
export const genreEQMappings = writable<GenreEQMapping[]>([]);

/** The id of the profile currently being edited in the EQ editor UI. */
export const editingProfileId = writable<string | null>(null);

/** Derived: the profile currently being edited (or null). */
export const editingProfile = derived(
	[eqProfiles, editingProfileId],
	([$profiles, $id]) => $profiles.find((p) => p.id === $id) ?? null
);

// ──────────────────────────────────────────────────────────────
// Load helpers
// ──────────────────────────────────────────────────────────────

/** Fetch profiles and genre mappings from the server and populate stores. */
export async function loadEQProfiles(): Promise<void> {
	const [profiles, mappings] = await Promise.all([
		eqApi.listProfiles(),
		eqApi.listGenreMappings()
	]);
	eqProfiles.set(profiles);
	genreEQMappings.set(mappings);

	// Apply the default profile immediately so the engine is in sync.
	const defaultProfile = profiles.find((p) => p.is_default) ?? null;
	if (defaultProfile) {
		applyEQProfile(defaultProfile);
	} else {
		disableEQ();
	}
}

// ──────────────────────────────────────────────────────────────
// Apply / disable
// ──────────────────────────────────────────────────────────────

/** Push a profile's bands into the audio engine and record it as active. */
export function applyEQProfile(profile: EQProfile): void {
	activeEQProfile.set(profile);
	audioEngine.setEQ(profile.bands);
	nativeSyncEQ(true, profile.bands);
}

/** Restore flat EQ (all gains 0) without changing which profile is saved. */
export function disableEQ(): void {
	activeEQProfile.set(null);
	const flat = DEFAULT_EQ_BANDS.map((b) => ({ ...b, gain: 0 }));
	audioEngine.setEQ(flat);
	nativeSyncEQ(false, flat);
}

// ──────────────────────────────────────────────────────────────
// Profile CRUD (wraps API + updates stores)
// ──────────────────────────────────────────────────────────────

export async function createEQProfile(name: string, bands: EQBand[]): Promise<EQProfile> {
	const profile = await eqApi.createProfile({ name, bands });
	eqProfiles.update((list) => [...list, profile]);
	return profile;
}

export async function saveEQProfile(id: string, name: string, bands: EQBand[]): Promise<EQProfile> {
	const updated = await eqApi.updateProfile(id, { name, bands });
	eqProfiles.update((list) => list.map((p) => (p.id === id ? updated : p)));
	// If this was the active profile, re-apply it so audio engine is in sync.
	if (get(activeEQProfile)?.id === id) {
		applyEQProfile(updated);
	}
	return updated;
}

export async function removeEQProfile(id: string): Promise<void> {
	await eqApi.deleteProfile(id);
	eqProfiles.update((list) => list.filter((p) => p.id !== id));
	if (get(activeEQProfile)?.id === id) {
		disableEQ();
	}
	if (get(editingProfileId) === id) {
		editingProfileId.set(null);
	}
}

export async function setDefaultEQProfile(id: string): Promise<void> {
	await eqApi.setDefaultProfile(id);
	eqProfiles.update((list) =>
		list.map((p) => ({ ...p, is_default: p.id === id }))
	);
}

// ──────────────────────────────────────────────────────────────
// Genre mapping helpers
// ──────────────────────────────────────────────────────────────

export async function setGenreEQMapping(genreId: string, profileId: string): Promise<void> {
	await eqApi.setGenreMapping(genreId, profileId);
	genreEQMappings.update((list) => {
		const existing = list.findIndex((m) => m.genre_id === genreId);
		if (existing >= 0) {
			return list.map((m) => (m.genre_id === genreId ? { ...m, profile_id: profileId } : m));
		}
		return [...list, { user_id: '', genre_id: genreId, profile_id: profileId }];
	});
}

export async function removeGenreEQMapping(genreId: string): Promise<void> {
	await eqApi.deleteGenreMapping(genreId);
	genreEQMappings.update((list) => list.filter((m) => m.genre_id !== genreId));
}

/**
 * Look up which EQ profile (if any) should be applied for a given genre id.
 * Returns the profile if found, or null to keep the current setting.
 */
export function getProfileForGenre(genreId: string): EQProfile | null {
	const mappings = get(genreEQMappings);
	const profiles = get(eqProfiles);
	const mapping = mappings.find((m) => m.genre_id === genreId);
	if (!mapping) return null;
	return profiles.find((p) => p.id === mapping.profile_id) ?? null;
}
