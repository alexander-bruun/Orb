import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { VitePWA } from 'vite-plugin-pwa';

// TAURI_ENV_ARCH is set automatically by the Tauri CLI during dev and build.
const isTauriBuild = !!process.env.TAURI_ENV_ARCH;
const isCapacitorBuild = !!process.env.CAPACITOR_PLATFORM;
console.log('[vite] TAURI_ENV_ARCH =', process.env.TAURI_ENV_ARCH, '→ isTauri:', isTauriBuild);
console.log('[vite] CAPACITOR_PLATFORM =', process.env.CAPACITOR_PLATFORM, '→ isCapacitor:', isCapacitorBuild);

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		// Service worker only makes sense in the web build; Tauri ships its own
		// update mechanism and doesn't benefit from a PWA SW.
		...(!isTauriBuild && !isCapacitorBuild ? [
			VitePWA({
				registerType: 'autoUpdate',
				strategies: 'injectManifest',
				srcDir: 'src',
				filename: 'sw.ts',
				manifest: {
					name: 'Orb',
					short_name: 'Orb',
					description: 'Self-hosted music streaming',
					start_url: '/',
					display: 'standalone',
					background_color: '#080809',
					theme_color: '#c084fc',
					icons: [
						{ src: '/pwa-192.png', sizes: '192x192', type: 'image/png' },
						{ src: '/pwa-512.png', sizes: '512x512', type: 'image/png', purpose: 'any maskable' },
					],
				},
				injectManifest: {
					// Cache the app shell and static assets
					globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
				},
				devOptions: { enabled: false },
			}),
		] : []),
	],
	define: {
		__IS_CAPACITOR__: isCapacitorBuild,
	},
	build: {
		rollupOptions: {},
	},
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				rewrite: (path) => path.replace(/^\/api/, ''),
				changeOrigin: true,
				ws: true,
			}
		}
	}
});
