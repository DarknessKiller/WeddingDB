import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({ fallback: 'index.html' })
		})
	],
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
