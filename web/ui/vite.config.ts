import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

const isTauriBuild = !!process.env.TAURI_ENV_PLATFORM;
console.log('[vite] TAURI_ENV_PLATFORM =', process.env.TAURI_ENV_PLATFORM, '→ isTauri:', isTauriBuild);

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	define: {
		__IS_TAURI__: isTauriBuild,
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
