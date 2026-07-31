import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit()
	],
	build: {
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (id.includes('konva') || id.includes('svelte-konva')) return 'konva';
					if (id.includes('node_modules/zod')) return 'zod';
				}
			}
		}
	},
	server: {
		host: '0.0.0.0',
		proxy: {
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			},
			'/uploads': {
				target: 'http://localhost:8080',
				changeOrigin: true
			}
		}
	}
});
