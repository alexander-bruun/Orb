import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { STORAGE_KEYS } from '$lib/constants';

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

const STORAGE_KEY = STORAGE_KEYS.THEME;
const AVATAR_KEY  = STORAGE_KEYS.AVATAR;

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

// ── Waveform seek bar preference ──────────────────────────────────────────────

const WAVEFORM_KEY = STORAGE_KEYS.WAVEFORM_ENABLED;

function createWaveformStore() {
	const initial = browser
		? (localStorage.getItem(WAVEFORM_KEY) ?? 'true') !== 'false'
		: true;
	const { subscribe, set } = writable<boolean>(initial);
	return {
		subscribe,
		set(value: boolean) {
			if (browser) localStorage.setItem(WAVEFORM_KEY, String(value));
			set(value);
		},
		toggle() {
			const next = !(browser ? (localStorage.getItem(WAVEFORM_KEY) ?? 'true') !== 'false' : true);
			if (browser) localStorage.setItem(WAVEFORM_KEY, String(next));
			set(next);
		}
	};
}

export const waveformEnabled = createWaveformStore();

// ── Visualizer button visibility ──────────────────────────────────────────────

const VIS_BTN_KEY = STORAGE_KEYS.VISUALIZER_BUTTON_ENABLED;

function createVisualizerButtonStore() {
	const initial = browser
		? (localStorage.getItem(VIS_BTN_KEY) ?? 'true') !== 'false'
		: true;
	const { subscribe, set } = writable<boolean>(initial);
	return {
		subscribe,
		set(value: boolean) {
			if (browser) localStorage.setItem(VIS_BTN_KEY, String(value));
			set(value);
		},
		toggle() {
			const current = browser ? (localStorage.getItem(VIS_BTN_KEY) ?? 'true') !== 'false' : true;
			const next = !current;
			if (browser) localStorage.setItem(VIS_BTN_KEY, String(next));
			set(next);
		}
	};
}

export const visualizerButtonEnabled = createVisualizerButtonStore();

// ── Bottom bar secondary info preference ─────────────────────────────────────

const BOTTOM_BAR_SECONDARY_KEY = STORAGE_KEYS.BOTTOM_BAR_SECONDARY;

export type BottomBarSecondary = 'album' | 'artist';

function createBottomBarSecondaryStore() {
	const initial: BottomBarSecondary = browser
		? ((localStorage.getItem(BOTTOM_BAR_SECONDARY_KEY) ?? 'album') as BottomBarSecondary)
		: 'album';
	const { subscribe, set } = writable<BottomBarSecondary>(initial);
	return {
		subscribe,
		set(value: BottomBarSecondary) {
			if (browser) localStorage.setItem(BOTTOM_BAR_SECONDARY_KEY, value);
			set(value);
		},
	};
}

export const bottomBarSecondary = createBottomBarSecondaryStore();

// ── Listen Along button visibility ───────────────────────────────────────────

const LISTEN_ALONG_KEY = STORAGE_KEYS.LISTEN_ALONG_ENABLED;

function createListenAlongEnabledStore() {
	const initial = browser
		? (localStorage.getItem(LISTEN_ALONG_KEY) ?? 'true') !== 'false'
		: true;
	const { subscribe, set } = writable<boolean>(initial);
	return {
		subscribe,
		set(value: boolean) {
			if (browser) localStorage.setItem(LISTEN_ALONG_KEY, String(value));
			set(value);
		},
		toggle() {
			const current = browser ? (localStorage.getItem(LISTEN_ALONG_KEY) ?? 'true') !== 'false' : true;
			const next = !current;
			if (browser) localStorage.setItem(LISTEN_ALONG_KEY, String(next));
			set(next);
		}
	};
}

export const listenAlongEnabled = createListenAlongEnabledStore();
