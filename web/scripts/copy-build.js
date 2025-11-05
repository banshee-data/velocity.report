#!/usr/bin/env node
import { cpSync, existsSync, rmSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const projectRoot = join(__dirname, '..');
const staticDir = join(projectRoot, '..', 'static');
const buildDir = join(projectRoot, 'build');
const appStaticDir = join(staticDir, 'app');

console.log('Copying build to static directory...');

// Remove old app directory if it exists
if (existsSync(appStaticDir)) {
	console.log('Removing old static/app directory...');
	rmSync(appStaticDir, { recursive: true, force: true });
}

// Copy build contents to static
if (existsSync(buildDir)) {
	console.log(`Copying from ${buildDir} to ${staticDir}...`);
	cpSync(buildDir, staticDir, { recursive: true });
	console.log('Build copied successfully!');
} else {
	console.error('Error: build directory does not exist!');
	process.exit(1);
}
