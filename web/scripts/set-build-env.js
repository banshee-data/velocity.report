#!/usr/bin/env node
import { execSync, spawn } from 'child_process';
import { readFileSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

// Get git SHA (short form)
let gitSha = process.env.PUBLIC_GIT_SHA;
if (!gitSha) {
	try {
		gitSha = execSync('git rev-parse --short HEAD', { encoding: 'utf8' }).trim();
	} catch (error) {
		console.warn('Warning: Could not get git SHA:', error.message);
		gitSha = 'unknown';
	}
}

// Get build time (ISO format) or 'DEV_MODE' for dev server
let buildTime = process.env.PUBLIC_BUILD_TIME;
if (!buildTime) {
	buildTime = process.argv[2] === 'dev' ? 'DEV_MODE' : new Date().toISOString();
}

// Get build version (from Makefile VERSION via environment)
let buildVersion = process.env.PUBLIC_BUILD_VERSION;
if (!buildVersion) {
	buildVersion = 'unknown';
}

// Get web version (from package.json — canonical source)
const __dirname = dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(readFileSync(join(__dirname, '..', 'package.json'), 'utf8'));
const webVersion = pkg.version || 'unknown';

// Set environment variables
process.env.PUBLIC_GIT_SHA = gitSha;
process.env.PUBLIC_BUILD_TIME = buildTime;
process.env.PUBLIC_BUILD_VERSION = buildVersion;
process.env.PUBLIC_WEB_VERSION = webVersion;

// Get the command to run (everything after the first argument)
const args = process.argv.slice(3);
if (args.length === 0) {
	console.error('Error: No command specified');
	process.exit(1);
}

// Run the command (first arg is the command, rest are arguments)
const [command, ...commandArgs] = args;
const child = spawn(command, commandArgs, {
	stdio: 'inherit',
	env: process.env,
	shell: false
});

child.on('exit', (code) => {
	process.exit(code ?? 0);
});
