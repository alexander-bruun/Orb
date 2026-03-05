import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { VitePWA } from 'vite-plugin-pwa';

const isTauriBuild = !!process.env.TAURI_ENV_PLATFORM;
console.log('[vite] TAURI_ENV_PLATFORM =', process.env.TAURI_ENV_PLATFORM, '→ isTauri:', isTauriBuild);

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit(),
		// Service worker only makes sense in the web build; Tauri ships its own
		// installer mechanism and doesn't benefit from a PWA SW.
		...(!isTauriBuild ? [
			VitePWA({
				registerType: 'autoUpdate',
				strategies: 'generateSW',
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
				workbox: {
					// Cache the app shell and static assets
					globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
					// Don't try to precache audio streams or API responses
					navigationPreload: false,
					runtimeCaching: [
						{
							// Cache cover art with a stale-while-revalidate strategy
							urlPattern: /\/api\/(covers|artists)\//,
							handler: 'CacheFirst',
							options: {
								cacheName: 'cover-art',
								expiration: { maxEntries: 500, maxAgeSeconds: 60 * 60 * 24 * 30 },
							},
						},
					],
				},
				devOptions: { enabled: false },
			}),
		] : []),
	],
	define: {
		__IS_TAURI__: isTauriBuild,
	},
	build: {
		rollupOptions: {
			// Tauri API packages are only available inside the Tauri desktop shell.
			// When building the web/Docker image they must be left as external so
			// Rollup does not attempt to resolve them (they are only imported at
			// runtime behind an `if (isTauri())` guard).
			external: isTauriBuild ? [] : [/^@tauri-apps\//],
		},
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
