#!/usr/bin/env node
// Docs/code drift lint. Fails when a doc names a repo path or a function that
// does not exist, or gives an ADR an ambiguous Status.
import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname;
const CODE_ROOTS = ['frontend/src', 'backend/internal', 'scripts'];

function walk(dir, out = []) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) walk(p, out);
		else out.push(p);
	}
	return out;
}

const allow = new Set(
	(existsSync(join(ROOT, 'scripts/docs-allow.txt'))
		? readFileSync(join(ROOT, 'scripts/docs-allow.txt'), 'utf8')
		: ''
	).split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'))
);

const codeFiles = CODE_ROOTS.flatMap((r) => walk(join(ROOT, r), []));
const codeText = codeFiles.map((f) => readFileSync(f, 'utf8')).join('\n');

const PATH_TOKEN = /^[\w./\[\]@{}+-]+\.(ts|svelte|go|js|mjs|json|yml|yaml|sh|sql|example)$/;
const DECL = /^(?:export\s+)?(?:async\s+)?(?:function|type|interface|const|class)\s+([A-Za-z_]\w*)$/;
const GO_DECL = /^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)$/;
const STATUS = /^(Proposed|Accepted — not implemented|Implemented:)/;
const PATH_ROOTS = ['docs/', 'frontend/', 'backend/', 'scripts/', '.github/'];

const problems = [];

for (const file of walk(join(ROOT, 'docs'), []).filter((f) => f.endsWith('.md'))) {
	const rel = file.slice(ROOT.length);
	const text = readFileSync(file, 'utf8');

	for (const [, raw] of text.matchAll(/`([^`\n]+)`/g)) {
		const token = raw.trim();
		if (allow.has(token)) continue;

		if (PATH_TOKEN.test(token) && PATH_ROOTS.some((r) => token.startsWith(r))) {
			if (!existsSync(join(ROOT, token))) {
				problems.push(`${rel}: path not found \`${token}\``);
			}
			continue;
		}

		const name = token.match(DECL)?.[1] ?? token.match(GO_DECL)?.[1];
		if (name && !new RegExp(`\\b${name}\\b`).test(codeText)) {
			problems.push(`${rel}: declaration not found in code \`${token}\``);
		}

		// Call-shaped tokens: `getQueue(wid)` in prose must name a declared function.
		const call = token.match(/^([A-Za-z_]\w*)\(/)?.[1];
		if (!name && call && !new RegExp(`(?:function\\s+|const\\s+|let\\s+|func\\s+(?:\\([^)]*\\)\\s*)?)${call}\\b`).test(codeText)) {
			problems.push(`${rel}: function not found in code \`${token}\``);
		}
	}

	if (rel.startsWith('docs/adr/')) {
		const lines = text.split('\n');
		const at = lines.findIndex((l) => l.trim() === '## Status' || l.startsWith('Status:'));
		if (at === -1) {
			problems.push(`${rel}: missing Status`);
			continue;
		}
		const status =
			lines[at].startsWith('Status:')
				? lines[at].slice('Status:'.length).trim()
				: lines.slice(at + 1).find((l) => l.trim())?.trim() ?? '';
		if (!STATUS.test(status)) {
			problems.push(`${rel}: Status must start with one of ${STATUS.source} — got "${status.slice(0, 60)}"`);
		} else if (status.startsWith('Implemented:')) {
			const paths = status.match(/`[^`]+`|[\w./\[\]@-]+\.(?:ts|svelte|go|md)/g) ?? [];
			if (!paths.some((p) => existsSync(join(ROOT, p.replace(/`/g, ''))))) {
				problems.push(`${rel}: Implemented status names no existing path`);
			}
		}
	}
}

if (problems.length) {
	console.error(problems.join('\n'));
	console.error(`\n${problems.length} doc problem(s). Fix the doc, or add the token to scripts/docs-allow.txt if it is historical.`);
	process.exit(1);
}
console.log('docs ok');
