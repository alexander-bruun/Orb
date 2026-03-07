import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

export const ACCENTS = [
	{ name: 'Purple', value: '#c084fc', rgb: '192,132,252' },
	{ name: 'Blue',   value: '#60a5fa', rgb: '96,165,250' },
	{ name: 'Green',  value: '#4ade80', rgb: '74,222,128' },
	{ name: 'Orange', value: '#fb923c', rgb: '251,146,60' },
	{ name: 'Pink',   value: '#f472b6', rgb: '244,114,182' },
	{ name: 'Cyan',   value: '#22d3ee', rgb: '34,211,238' },
	{ name: 'Red',    value: '#f87171', rgb: '248,113,113' },
	{ name: 'Yellow', value: '#facc15', rgb: '250,204,21' },
] as const;

export type AccentColor = typeof ACCENTS[number];

interface ThemeState {
	mode: 'dark' | 'light';
	accent: string;
}

const STORAGE_KEY = 'orb_theme';
const AVATAR_KEY  = 'orb_avatar';

function loadTheme(): ThemeState {
	if (!browser) return { mode: 'dark', accent: '#c084fc' };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		return raw ? JSON.parse(raw) : { mode: 'dark', accent: '#c084fc' };
	} catch {
		return { mode: 'dark', accent: '#c084fc' };
	}
}

function applyTheme(state: ThemeState) {
	if (!browser) return;
	const html = document.documentElement;
	html.setAttribute('data-theme', state.mode);

	const accent = ACCENTS.find(a => a.value === state.accent) ?? ACCENTS[0];
	html.style.setProperty('--accent',      accent.value);
	html.style.setProperty('--accent-dim',  `rgba(${accent.rgb},0.12)`);
	html.style.setProperty('--accent-glow', `rgba(${accent.rgb},0.25)`);
}

function saveTheme(state: ThemeState) {
	if (browser) localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

function createThemeStore() {
	const initial = loadTheme();
	const { subscribe, set, update } = writable<ThemeState>(initial);

	if (browser) applyTheme(initial);

	return {
		subscribe,
		init() {
			const state = get({ subscribe });
			applyTheme(state);
		},
		setMode(mode: 'dark' | 'light') {
			update(s => {
				const next = { ...s, mode };
				saveTheme(next);
				applyTheme(next);
				return next;
			});
		},
		setAccent(accent: string) {
			update(s => {
				const next = { ...s, accent };
				saveTheme(next);
				applyTheme(next);
				return next;
			});
		}
	};
}

export const themeStore = createThemeStore();

// ── Avatar store ──────────────────────────────────────────────────────────────

function createAvatarStore() {
	const initial = browser ? (localStorage.getItem(AVATAR_KEY) ?? null) : null;
	const { subscribe, set } = writable<string | null>(initial);

	return {
		subscribe,
		set(dataUrl: string) {
			if (browser) localStorage.setItem(AVATAR_KEY, dataUrl);
			set(dataUrl);
		},
		clear() {
			if (browser) localStorage.removeItem(AVATAR_KEY);
			set(null);
		}
	};
}

export const avatarStore = createAvatarStore();
