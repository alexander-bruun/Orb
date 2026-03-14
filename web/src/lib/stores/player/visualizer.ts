/**
 * Visualizer store — state management for the sound visualizer widget.
 *
 * Persists user preferences to localStorage so they survive page reloads.
 */
import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type VisualizerType = 'spectrum' | 'waveform' | 'track-waveform' | 'spectrogram';

/** Named anchor positions for the floating widget. */
export type VisualizerPosition =
	| 'bottom-right'
	| 'bottom-center'
	| 'bottom-left'
	| 'top-right'
	| 'top-center'
	| 'top-left';

export type VisualizerColorScheme = 'accent' | 'rainbow' | 'mono';

export interface VisualizerState {
	/** Whether the widget is currently shown. */
	visible: boolean;
	/** Which visualizer mode is active. */
	type: VisualizerType;
	/** Anchor position preset. */
	position: VisualizerPosition;
	/** Colour palette for the visualizer canvas. */
	colorScheme: VisualizerColorScheme;
	/**
	 * Free drag offset (px) from the anchor position.
	 * Reset to { x: 0, y: 0 } when the user switches position presets.
	 */
	dragOffset: { x: number; y: number };
}

const STORAGE_KEY = 'orb-visualizer-prefs';

function loadPrefs(): Partial<Omit<VisualizerState, 'dragOffset'>> {
	if (!browser) return {};
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		return raw ? JSON.parse(raw) : {};
	} catch {
		return {};
	}
}

function savePrefs(state: VisualizerState): void {
	if (!browser) return;
	try {
		// Don't persist the transient dragOffset — intentional.
		const { dragOffset: _skip, ...persistable } = state;
		void _skip;
		localStorage.setItem(STORAGE_KEY, JSON.stringify(persistable));
	} catch {
		/* storage unavailable */
	}
}

const defaults: VisualizerState = {
	visible: false,
	type: 'spectrum',
	position: 'bottom-right',
	colorScheme: 'accent',
	dragOffset: { x: 0, y: 0 },
};

const saved = loadPrefs();

function createVisualizerStore() {
	const initial: VisualizerState = { ...defaults, ...saved };
	const { subscribe, update } = writable<VisualizerState>(initial);

	return {
		subscribe,

		toggle() {
			update((s) => {
				const next = { ...s, visible: !s.visible };
				savePrefs(next);
				return next;
			});
		},

		setVisible(visible: boolean) {
			update((s) => {
				const next = { ...s, visible };
				savePrefs(next);
				return next;
			});
		},

		setType(type: VisualizerType) {
			update((s) => {
				const next = { ...s, type };
				savePrefs(next);
				return next;
			});
		},

		setPosition(position: VisualizerPosition) {
			update((s) => {
				// Reset drag offset when snapping to a new preset.
				const next = { ...s, position, dragOffset: { x: 0, y: 0 } };
				savePrefs(next);
				return next;
			});
		},

		setColorScheme(colorScheme: VisualizerColorScheme) {
			update((s) => {
				const next = { ...s, colorScheme };
				savePrefs(next);
				return next;
			});
		},

		/** Update the free-drag offset (not persisted). */
		setDragOffset(x: number, y: number) {
			update((s) => ({ ...s, dragOffset: { x, y } }));
		},
	};
}

export const visualizerStore = createVisualizerStore();
