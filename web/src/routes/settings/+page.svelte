<script lang="ts">
	import {
		createSerialConfig,
		deleteSerialConfig,
		disableTailscale,
		enableTailscale,
		getConfig,
		getSensorModels,
		getSerialConfigs,
		getSerialDevices,
		getTailscaleStatus,
		getTransitWorkerState,
		testSerialPort,
		updateSerialConfig,
		updateTransitWorker,
		type Config,
		type SerialConfig,
		type SerialConfigRequest,
		type SerialDevice,
		type SerialTestResponse,
		type SensorModel,
		type TailscaleStatus,
		type TransitRunInfo,
		type TransitWorkerState
	} from '$lib/api';
	import { paperSize, initializePaperSize, updatePaperSize } from '$lib/stores/paper';
	import QRCode from 'qrcode';
	import { displayTimezone, initializeTimezone, updateTimezone } from '$lib/stores/timezone';
	import { displayUnits, initializeUnits, updateUnits } from '$lib/stores/units';
	import { AVAILABLE_PAPER_SIZES, getPaperLabel, type PaperSize } from '$lib/paper';
	import { AVAILABLE_TIMEZONES, getTimezoneLabel, type Timezone } from '$lib/timezone';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '$lib/units';
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import {
		Button,
		Checkbox,
		Dialog,
		Field,
		Notification,
		SelectField,
		Switch,
		TextField
	} from 'svelte-ux';

	// ─── Display preferences (units, timezone, paper size) ───────────────
	let config: Config = { units: 'mph', timezone: 'UTC' };
	let selectedUnits: Unit = 'mph';
	let selectedTimezone: Timezone = 'UTC';
	let selectedPaperSize: PaperSize = 'a4';
	let loading = true;
	let message = '';

	// ─── Transit worker ─────────────────────────────────────────────────
	let transitWorkerEnabled = true;
	let transitWorkerLoading = false;
	let transitWorkerStatus: TransitWorkerState | null = null;
	let currentRun: TransitRunInfo | null = null;
	let resolvedLastRun: TransitRunInfo | null = null;
	let transitWorkerRefreshTimer: ReturnType<typeof setInterval> | null = null;
	let transitWorkerRefreshTimeouts: Array<ReturnType<typeof setTimeout>> = [];

	// ─── Tailscale (poll fast during login, slow once Connected) ─────────
	let tailscaleStatus: TailscaleStatus | null = null;
	let tailscaleEnabled = false;
	let tailscaleConnected = false;
	let tailscaleLoading = false;
	let tailscaleError = '';
	let tailscaleQrDataUrl = '';
	let lastQrUrl = '';
	let tailscalePollTimer: ReturnType<typeof setInterval> | null = null;
	let tailscalePollFastInterval = false;

	// ─── Serial port configuration (lifted to page level so the editor
	//     can render as the right column of the vr-page split layout
	//     instead of as a fixed overlay) ──────────────────────────────────
	let serialConfigs: SerialConfig[] = [];
	let availableDevices: SerialDevice[] = [];
	let portPathOptions: { value: string; label: string }[] = [];
	let sensorModelOptions: { value: string; label: string }[] = [];
	let serialMessage = '';
	let serialMessageType: 'success' | 'error' | 'info' = 'info';
	let showEditPanel = false;
	let editingConfig: SerialConfig | null = null;
	let formData: SerialConfigRequest = {
		name: '',
		port_path: '',
		baud_rate: 19200,
		data_bits: 8,
		stop_bits: 1,
		parity: 'N',
		enabled: true,
		description: '',
		sensor_model: 'ops243-a'
	};
	let manualPortEntry = false;
	let rescanning = false;
	let testing = false;
	let testResult: SerialTestResponse | null = null;
	let showTestResultDialog = false;
	let deletingConfig: SerialConfig | null = null;
	let showDeleteDialog = false;

	const baudRateOptions = [9600, 19200, 38400, 57600, 115200].map((rate) => ({
		value: rate,
		label: rate.toString()
	}));
	const dataBitsOptions = [5, 6, 7, 8].map((n) => ({ value: n, label: n.toString() }));
	const stopBitsOptions = [1, 2].map((n) => ({ value: n, label: n.toString() }));
	const parityOptions = [
		{ value: 'N', label: 'None' },
		{ value: 'E', label: 'Even' },
		{ value: 'O', label: 'Odd' }
	];

	function formatTimestamp(value?: string) {
		if (!value || value.startsWith('0001-01-01')) {
			return 'N/A';
		}
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) {
			return 'N/A';
		}
		try {
			return new Intl.DateTimeFormat('en-GB', {
				timeZone: $displayTimezone,
				dateStyle: 'medium',
				timeStyle: 'medium'
			}).format(date);
		} catch {
			return date.toLocaleString();
		}
	}

	function formatDuration(durationMs?: number) {
		if (durationMs === undefined || durationMs === null) {
			return 'N/A';
		}
		const totalSeconds = Math.max(0, Math.round(durationMs / 1000));
		const minutes = Math.floor(totalSeconds / 60);
		const seconds = totalSeconds % 60;
		if (minutes > 0) {
			return `${minutes}m ${seconds}s`;
		}
		return `${seconds}s`;
	}

	function formatTrigger(trigger?: string) {
		if (!trigger) return 'Unknown';
		switch (trigger) {
			case 'initial':
				return 'Initial';
			case 'periodic':
				return 'Scheduled';
			case 'manual':
				return 'Manual';
			case 'full-history':
				return 'Full history';
			default:
				return trigger;
		}
	}

	function getRunError(run?: TransitRunInfo | null) {
		return run?.error || undefined;
	}

	function resolveLastRun(status: TransitWorkerState | null): TransitRunInfo | null {
		if (!status) return null;
		if (status.last_run) return status.last_run;
		if (status.last_run_at && !status.last_run_at.startsWith('0001-01-01')) {
			return {
				started_at: status.last_run_at,
				finished_at: status.last_run_at,
				error: status.last_run_error
			};
		}
		return null;
	}

	function scheduleTransitWorkerRefresh() {
		transitWorkerRefreshTimeouts.forEach((id) => clearTimeout(id));
		transitWorkerRefreshTimeouts = [];
		transitWorkerRefreshTimeouts.push(setTimeout(loadTransitWorkerState, 1200));
		transitWorkerRefreshTimeouts.push(setTimeout(loadTransitWorkerState, 5000));
	}

	function stopTransitWorkerRefresh() {
		if (transitWorkerRefreshTimer) {
			clearInterval(transitWorkerRefreshTimer);
			transitWorkerRefreshTimer = null;
		}
		transitWorkerRefreshTimeouts.forEach((id) => clearTimeout(id));
		transitWorkerRefreshTimeouts = [];
	}

	$: selectedUnits = $displayUnits;
	$: selectedTimezone = $displayTimezone;
	$: selectedPaperSize = $paperSize;
	$: currentRun = transitWorkerStatus?.current_run ?? null;
	$: resolvedLastRun = resolveLastRun(transitWorkerStatus);

	$: if (selectedUnits && selectedUnits !== $displayUnits && !loading) {
		handleUnitsChange(selectedUnits);
	}
	$: if (selectedTimezone && selectedTimezone !== $displayTimezone && !loading) {
		handleTimezoneChange(selectedTimezone);
	}
	$: if (selectedPaperSize && selectedPaperSize !== $paperSize && !loading) {
		handlePaperSizeChange(selectedPaperSize);
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
			initializeTimezone(config.timezone);
			initializePaperSize();
		} catch (e) {
			console.error('Could not load configuration:', e);
			message = 'Could not load the configuration. Check the server is running.';
		} finally {
			loading = false;
		}
	}

	async function loadTransitWorkerState() {
		try {
			const state = await getTransitWorkerState();
			transitWorkerEnabled = state.enabled;
			transitWorkerStatus = state;
		} catch (e) {
			console.error('Could not load transit worker state:', e);
		}
	}

	async function loadTailscaleStatus() {
		if (tailscaleLoading) return;
		try {
			const next = await getTailscaleStatus();
			tailscaleStatus = next;
			syncTailscaleState(next);
			tuneTailscalePolling(next);
			updateTailscaleQr(next.login_url);
		} catch (e) {
			console.error('Could not load Tailscale status:', e);
		}
	}

	function tuneTailscalePolling(status: TailscaleStatus | null) {
		const wantsFast =
			!!status &&
			(status.login_in_progress ||
				!!status.login_url ||
				status.backend_state === 'NeedsLogin' ||
				status.backend_state === 'Starting');
		if (wantsFast === tailscalePollFastInterval && tailscalePollTimer) return;
		tailscalePollFastInterval = wantsFast;
		if (tailscalePollTimer) clearInterval(tailscalePollTimer);
		tailscalePollTimer = setInterval(loadTailscaleStatus, wantsFast ? 2000 : 30000);
	}

	async function handleTailscaleToggle(enabled: boolean) {
		tailscaleLoading = true;
		tailscaleError = '';
		try {
			tailscaleStatus = enabled ? await enableTailscale() : await disableTailscale();
			syncTailscaleState(tailscaleStatus);
			tuneTailscalePolling(tailscaleStatus);
			updateTailscaleQr(tailscaleStatus.login_url);
		} catch (e) {
			console.error('Could not update Tailscale:', e);
			tailscaleError = (e as Error).message || 'Could not update Tailscale.';
		} finally {
			tailscaleLoading = false;
		}
	}

	async function copyLoginUrl() {
		if (!tailscaleStatus?.login_url) return;
		try {
			await navigator.clipboard.writeText(tailscaleStatus.login_url);
			message = 'Login URL copied to clipboard.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('clipboard write failed:', e);
		}
	}

	async function updateTailscaleQr(url: string | undefined) {
		if (!url) {
			tailscaleQrDataUrl = '';
			lastQrUrl = '';
			return;
		}
		if (url === lastQrUrl) return;
		lastQrUrl = url;
		try {
			tailscaleQrDataUrl = await QRCode.toDataURL(url, { width: 220, margin: 1 });
		} catch (e) {
			console.error('QR encode failed:', e);
			tailscaleQrDataUrl = '';
		}
	}

	function syncTailscaleState(status: TailscaleStatus | null) {
		tailscaleEnabled = !!status?.daemon_running;
		tailscaleConnected = status?.backend_state === 'Running';
	}

	async function handleTransitWorkerToggle(enabled: boolean) {
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ enabled, trigger: enabled });
			transitWorkerEnabled = response.enabled;
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = enabled ? 'Transit worker enabled: run started.' : 'Transit worker disabled.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not update transit worker:', e);
			message = 'Could not update the transit worker. Try again shortly.';
			transitWorkerEnabled = !enabled;
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunNow() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker first.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Transit worker run started.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not trigger transit worker run:', e);
			message = 'Could not start the transit worker run.';
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunFullHistory() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker first.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger_full_history: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Full history reprocessing started.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not start full history run:', e);
			message = 'Could not start full history reprocessing.';
		} finally {
			transitWorkerLoading = false;
		}
	}

	function handleUnitsChange(newUnits: Unit) {
		try {
			updateUnits(newUnits);
			message = 'Units updated.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not update units:', e);
			message = 'Could not save the units change.';
		}
	}

	function handlePaperSizeChange(newSize: PaperSize) {
		try {
			updatePaperSize(newSize);
			message = 'Paper size updated.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not update paper size:', e);
			message = 'Could not save the paper size change.';
		}
	}

	function handleTimezoneChange(newTimezone: Timezone) {
		try {
			updateTimezone(newTimezone);
			message = 'Timezone updated.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('Could not update timezone:', e);
			message = 'Could not save the timezone change.';
		}
	}

	// ─── Serial: load, rescan, save, delete, test ───────────────────────
	function showSerialMessage(msg: string, type: 'success' | 'error' | 'info' = 'info') {
		serialMessage = msg;
		serialMessageType = type;
		setTimeout(() => (serialMessage = ''), 5000);
	}

	function rebuildPortPathOptions(devices: SerialDevice[], configs: SerialConfig[]) {
		const seen = new SvelteSet<string>();
		devices.forEach((d) => seen.add(d.port_path));
		configs.forEach((c) => seen.add(c.port_path));
		portPathOptions = Array.from(seen)
			.sort()
			.map((path) => ({ value: path, label: path }));
	}

	function isDetectedPort(path: string): boolean {
		return portPathOptions.some((opt) => opt.value === path);
	}

	async function loadSerialData() {
		try {
			const [configs, models, devices] = await Promise.all([
				getSerialConfigs(),
				getSensorModels(),
				getSerialDevices()
			]);
			serialConfigs = configs;
			availableDevices = devices;
			rebuildPortPathOptions(devices, configs);
			sensorModelOptions = models.map((m: SensorModel) => ({
				value: m.slug,
				label: m.display_name
			}));
		} catch (e) {
			console.error('Failed to load serial data:', e);
			showSerialMessage('Failed to load serial configuration', 'error');
		}
	}

	async function rescanDevices() {
		try {
			rescanning = true;
			const devices = await getSerialDevices();
			availableDevices = devices;
			rebuildPortPathOptions(devices, serialConfigs);
			showSerialMessage(
				devices.length === 0
					? 'No serial devices detected.'
					: `Found ${devices.length} device${devices.length === 1 ? '' : 's'}.`,
				devices.length === 0 ? 'info' : 'success'
			);
		} catch (e) {
			console.error('Failed to rescan devices:', e);
			showSerialMessage('Failed to rescan serial devices', 'error');
		} finally {
			rescanning = false;
		}
	}

	function openCreatePanel() {
		editingConfig = null;
		const defaultPort =
			availableDevices.length > 0
				? availableDevices[0].port_path
				: portPathOptions.length > 0
					? portPathOptions[0].value
					: '';
		formData = {
			name: '',
			port_path: defaultPort,
			baud_rate: 19200,
			data_bits: 8,
			stop_bits: 1,
			parity: 'N',
			enabled: true,
			description: '',
			sensor_model: 'ops243-a'
		};
		manualPortEntry = !defaultPort || !isDetectedPort(defaultPort);
		showEditPanel = true;
	}

	function openEditPanel(c: SerialConfig) {
		editingConfig = c;
		formData = {
			name: c.name,
			port_path: c.port_path,
			baud_rate: c.baud_rate,
			data_bits: c.data_bits,
			stop_bits: c.stop_bits,
			parity: c.parity,
			enabled: c.enabled,
			description: c.description,
			sensor_model: c.sensor_model
		};
		manualPortEntry = !isDetectedPort(c.port_path);
		showEditPanel = true;
	}

	function closeEditPanel() {
		showEditPanel = false;
		editingConfig = null;
	}

	async function handleSerialSave() {
		try {
			if (editingConfig) {
				await updateSerialConfig(editingConfig.id, formData);
				showSerialMessage('Configuration updated successfully', 'success');
			} else {
				await createSerialConfig(formData);
				showSerialMessage('Configuration created successfully', 'success');
			}
			closeEditPanel();
			await loadSerialData();
		} catch (e) {
			console.error('Failed to save config:', e);
			showSerialMessage(`Failed to save configuration: ${e}`, 'error');
		}
	}

	function openDeleteDialog(c: SerialConfig) {
		deletingConfig = c;
		showDeleteDialog = true;
	}

	async function handleSerialDelete() {
		if (!deletingConfig) return;
		try {
			await deleteSerialConfig(deletingConfig.id);
			showSerialMessage('Configuration deleted successfully', 'success');
			showDeleteDialog = false;
			deletingConfig = null;
			await loadSerialData();
		} catch (e) {
			console.error('Failed to delete config:', e);
			showSerialMessage(`Failed to delete configuration: ${e}`, 'error');
		}
	}

	async function handleSerialTest() {
		try {
			testing = true;
			testResult = await testSerialPort({
				port_path: formData.port_path,
				baud_rate: formData.baud_rate,
				data_bits: formData.data_bits,
				stop_bits: formData.stop_bits,
				parity: formData.parity,
				timeout_seconds: 5,
				auto_correct_baud: true
			});
			if (testResult.baud_rate !== formData.baud_rate) {
				formData.baud_rate = testResult.baud_rate;
			}
			showTestResultDialog = true;
		} catch (e) {
			console.error('Failed to test serial port:', e);
			showSerialMessage(`Failed to test serial port: ${e}`, 'error');
		} finally {
			testing = false;
		}
	}

	onMount(() => {
		loadConfig();
		loadTransitWorkerState();
		loadTailscaleStatus();
		loadSerialData();
		transitWorkerRefreshTimer = setInterval(loadTransitWorkerState, 30000);
		return () => {
			stopTransitWorkerRefresh();
			if (tailscalePollTimer) {
				clearInterval(tailscalePollTimer);
				tailscalePollTimer = null;
			}
		};
	});
</script>

<svelte:head>
	<title>Settings 🚴 velocity.report</title>
	<meta
		name="description"
		content="Manage display preferences, serial ports, transit worker, and Tailscale"
	/>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Settings</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Display preferences, sensor serial ports, transit worker, and Tailscale
				</p>
			</div>
		</div>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<!-- LEFT: settings cards stacked, scroll independently -->
		<div class="flex-1 overflow-y-auto p-6">
			<div class="mx-auto max-w-3xl space-y-6">
				{#if loading}
					<div class="text-surface-content/50 py-12 text-center" role="status" aria-live="polite">
						Loading settings...
					</div>
				{:else}
					<section class="space-y-4">
						<h2
							class="text-surface-content border-surface-content/10 border-b pb-2 text-lg font-semibold"
						>
							Display Preferences
						</h2>
						<div class="space-y-4">
							<SelectField
								label="Speed units"
								bind:value={selectedUnits}
								options={AVAILABLE_UNITS}
								clearable={false}
							/>
							<SelectField
								label="Timezone"
								bind:value={selectedTimezone}
								options={AVAILABLE_TIMEZONES}
								clearable={false}
							/>
							<SelectField
								label="PDF paper size"
								bind:value={selectedPaperSize}
								options={AVAILABLE_PAPER_SIZES}
								clearable={false}
							/>
							<p class="text-surface-content/70 text-xs">
								Saved to this browser. Server defaults: {getUnitLabel(config.units as Unit)},
								{getTimezoneLabel(config.timezone as Timezone)}. Current paper:
								{getPaperLabel($paperSize)}.
							</p>
						</div>
					</section>

					<section class="space-y-4">
						<h2
							class="text-surface-content border-surface-content/10 border-b pb-2 text-lg font-semibold"
						>
							Sensor Serial Ports
						</h2>
						<div class="space-y-4">
							{#if serialMessage}
								<Notification
									title={serialMessageType === 'success'
										? 'Success'
										: serialMessageType === 'error'
											? 'Error'
											: 'Info'}
									description={serialMessage}
									variant={serialMessageType === 'error' ? 'fill' : 'default'}
									class={serialMessageType === 'success'
										? 'bg-success-50 text-success-900 border-success-200'
										: serialMessageType === 'error'
											? 'bg-danger-50 text-danger-900 border-danger-200'
											: 'bg-info-50 text-info-900 border-info-200'}
								/>
							{/if}

							<div class="flex items-center justify-between">
								<p class="text-surface-content/70 text-sm">
									Configure and test radar sensor serial port connections.
								</p>
								<Button on:click={openCreatePanel} variant="fill" color="primary">
									Add serial port
								</Button>
							</div>

							{#if serialConfigs.length === 0}
								<p class="text-surface-content/70 text-sm">No serial configurations found.</p>
							{:else}
								<div class="overflow-x-auto">
									<table class="w-full border-collapse">
										<thead>
											<tr class="border-b">
												<th class="px-4 py-2 text-left font-semibold">Name</th>
												<th class="px-4 py-2 text-left font-semibold">Port Path</th>
												<th class="px-4 py-2 text-left font-semibold">Baud Rate</th>
												<th class="px-4 py-2 text-left font-semibold">Status</th>
												<th class="px-4 py-2 text-left font-semibold">Actions</th>
											</tr>
										</thead>
										<tbody>
											{#each serialConfigs as row (row.id)}
												<tr class="hover:bg-surface-50 border-b transition-colors">
													<td class="px-4 py-2">{row.name}</td>
													<td class="px-4 py-2">{row.port_path}</td>
													<td class="px-4 py-2">{row.baud_rate}</td>
													<td class="px-4 py-2">
														{#if row.enabled}
															<span class="text-success-500 font-medium">Enabled</span>
														{:else}
															<span class="text-surface-content/50">Disabled</span>
														{/if}
													</td>
													<td class="px-4 py-2">
														<div class="flex gap-2">
															<Button
																on:click={() => openEditPanel(row)}
																size="sm"
																variant="outline"
															>
																Edit
															</Button>
															<Button
																on:click={() => openDeleteDialog(row)}
																size="sm"
																variant="outline"
																color="danger"
															>
																Delete
															</Button>
														</div>
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							{/if}
						</div>
					</section>

					{#if message}
						<div
							class="rounded px-4 py-3 text-sm {message.includes('Could not')
								? 'bg-red-50 text-red-600'
								: 'bg-green-50 text-green-700'}"
							role={message.includes('Could not') ? 'alert' : 'status'}
							aria-live="polite"
						>
							{message}
						</div>
					{/if}

					<section class="space-y-4">
						<h2
							class="text-surface-content border-surface-content/10 border-b pb-2 text-lg font-semibold"
						>
							Transit Worker
						</h2>
						<div class="space-y-4">
							<p class="text-surface-content/70 text-sm">
								The transit worker periodically processes raw radar data into vehicle transits. When
								enabled, it runs on a schedule configured by the server. Toggling it on will also
								trigger an immediate run.
							</p>

							<div class="flex flex-wrap items-center gap-3">
								<Switch
									checked={transitWorkerEnabled}
									disabled={transitWorkerLoading}
									on:change={(e) => handleTransitWorkerToggle((e as CustomEvent).detail.value)}
								/>
								<span class="text-sm">
									{transitWorkerEnabled ? 'Enabled (runs on schedule)' : 'Disabled'}
								</span>
								<div class="flex flex-wrap items-center gap-2">
									<Button
										variant="outline"
										on:click={handleTransitWorkerRunNow}
										disabled={transitWorkerLoading || !transitWorkerEnabled || !!currentRun}
									>
										Run now
									</Button>
									<Button
										variant="outline"
										on:click={handleTransitWorkerRunFullHistory}
										disabled={transitWorkerLoading || !transitWorkerEnabled || !!currentRun}
									>
										Run full history
									</Button>
								</div>
							</div>

							{#if transitWorkerLoading}
								<p class="text-surface-content/70 text-xs italic">Updating...</p>
							{/if}

							<div class="grid gap-4 text-sm md:grid-cols-2">
								<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
									<p class="text-surface-content/60 text-xs uppercase">Current Run</p>
									<p class="mt-1 font-medium">
										{transitWorkerStatus ? (currentRun ? 'Running' : 'Idle') : 'Unknown'}
									</p>
									<p class="text-surface-content/70 text-xs">
										Started: {formatTimestamp(currentRun?.started_at)}
									</p>
									<p class="text-surface-content/70 text-xs">
										Trigger: {formatTrigger(currentRun?.trigger)}
									</p>
								</div>

								<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
									<p class="text-surface-content/60 text-xs uppercase">Most Recent Run</p>
									{#if !transitWorkerStatus}
										<p class="mt-1 font-medium">Unknown</p>
									{:else if resolvedLastRun}
										<p class="mt-1 font-medium">
											{getRunError(resolvedLastRun) ? 'Failed' : 'Completed'}
										</p>
									{:else}
										<p class="mt-1 font-medium">No runs yet</p>
									{/if}
									<p class="text-surface-content/70 text-xs">
										Finished: {formatTimestamp(resolvedLastRun?.finished_at)}
									</p>
									<p class="text-surface-content/70 text-xs">
										Duration: {formatDuration(resolvedLastRun?.duration_ms)}
									</p>
									<p class="text-surface-content/70 text-xs">
										Trigger: {formatTrigger(resolvedLastRun?.trigger)}
									</p>
									{#if getRunError(resolvedLastRun)}
										<p class="text-xs text-red-600">
											Error: {getRunError(resolvedLastRun)}
										</p>
									{/if}
								</div>
							</div>
						</div>
					</section>

					<section class="space-y-4">
						<h2
							class="text-surface-content border-surface-content/10 border-b pb-2 text-lg font-semibold"
						>
							Tailscale
						</h2>
						<div class="space-y-4">
							<p class="text-surface-content/70 text-sm">
								Tailscale puts this device on your private tailnet so you can reach the web UI from
								anywhere and SSH in without opening any ports on your LAN. When enabled, the device
								phones home to Tailscale's coordination server; when disabled, it does not.
								Tailscale SSH and publishing the web UI on
								<code>https://&lt;hostname&gt;.&lt;tailnet&gt;.ts.net</code> are turned on automatically.
							</p>

							<div class="flex flex-wrap items-center gap-3">
								<Switch
									checked={tailscaleEnabled}
									disabled={tailscaleLoading}
									on:change={(e) => {
										const target = e.target as HTMLInputElement | null;
										if (!target) return;
										handleTailscaleToggle(target.checked);
									}}
								/>
								<span class="text-sm">
									{#if !tailscaleStatus}
										Loading...
									{:else if tailscaleConnected}
										Connected
									{:else if tailscaleEnabled}
										{tailscaleStatus.backend_state || 'Starting'}
									{:else}
										Disabled
									{/if}
								</span>
								{#if tailscaleLoading}
									<span class="text-surface-content/70 text-xs italic">Updating...</span>
								{/if}
							</div>

							{#if tailscaleError}
								<p class="text-xs text-red-600" role="alert">{tailscaleError}</p>
							{/if}

							{#if tailscaleEnabled && !tailscaleConnected && tailscaleStatus?.login_url}
								<div
									class="border-surface-content/20 bg-surface-100 grid gap-4 rounded border p-4 md:grid-cols-[auto_1fr]"
								>
									{#if tailscaleQrDataUrl}
										<img
											src={tailscaleQrDataUrl}
											alt="Tailscale login QR code"
											class="bg-white p-2"
											width="220"
											height="220"
										/>
									{/if}
									<div class="space-y-2">
										<p class="text-sm font-medium">Finish enrolment</p>
										<p class="text-surface-content/70 text-xs">
											Open the link below or scan the QR code on your phone. After signing in, this
											device joins your tailnet and the page will update on its own.
										</p>
										<!-- eslint-disable svelte/no-navigation-without-resolve -->
										<a
											href={tailscaleStatus.login_url}
											target="_blank"
											rel="noopener noreferrer"
											class="block text-sm break-all text-blue-600 underline"
										>
											{tailscaleStatus.login_url}
										</a>
										<Button variant="outline" on:click={copyLoginUrl}>Copy login URL</Button>
									</div>
								</div>
							{:else if tailscaleEnabled && !tailscaleConnected}
								<p class="text-surface-content/70 text-xs italic">
									Waiting for Tailscale to issue a login URL...
								</p>
							{/if}

							{#if tailscaleConnected}
								<div class="grid gap-4 text-sm md:grid-cols-2">
									<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
										<p class="text-surface-content/60 text-xs uppercase">MagicDNS</p>
										<p class="mt-1 font-medium break-all">
											{tailscaleStatus?.magic_dns || tailscaleStatus?.hostname || 'unknown'}
										</p>
										{#if tailscaleStatus?.magic_dns}
											<!-- eslint-disable svelte/no-navigation-without-resolve -->
											<a
												href={`https://${tailscaleStatus.magic_dns}`}
												target="_blank"
												rel="noopener noreferrer"
												class="text-xs text-blue-600 underline"
											>
												Open web UI on tailnet
											</a>
										{/if}
									</div>
									<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
										<p class="text-surface-content/60 text-xs uppercase">Tailnet</p>
										<p class="mt-1 font-medium">{tailscaleStatus?.tailnet_name || 'unknown'}</p>
										<p class="text-surface-content/70 text-xs">
											{tailscaleStatus?.peer_count ?? 0} peer(s) visible
										</p>
									</div>
								</div>

								<div class="text-surface-content/80 grid gap-2 text-xs sm:grid-cols-2">
									<div>
										<span class="font-medium">Tailscale SSH:</span>
										{#if tailscaleStatus?.ssh_enabled}
											<span class="text-green-700">enabled</span>
										{:else if tailscaleStatus?.ssh_error}
											<span class="text-red-600">failed — {tailscaleStatus.ssh_error}</span>
										{:else}
											<span class="text-surface-content/60">pending</span>
										{/if}
									</div>
									<div>
										<span class="font-medium">Web UI on tailnet:</span>
										{#if tailscaleStatus?.serve_published}
											<span class="text-green-700">published</span>
										{:else if tailscaleStatus?.serve_error}
											<span class="text-red-600">failed — {tailscaleStatus.serve_error}</span>
										{:else}
											<span class="text-surface-content/60">pending</span>
										{/if}
									</div>
								</div>
							{/if}
						</div>
					</section>
				{/if}
			</div>
		</div>

		<!-- RIGHT: serial port editor as a real column (not an overlay).
		     Same affordance as /app/lidar/replay-cases — main content stays
		     scrollable and interactive while the editor is open. -->
		{#if showEditPanel}
			<aside
				class="border-surface-content/10 bg-surface-100 flex w-full flex-none flex-col overflow-y-auto border-l sm:w-[400px]"
				aria-label="Serial port editor"
			>
				<header class="border-surface-content/10 flex items-center justify-between border-b p-4">
					<span class="text-lg font-semibold">
						{editingConfig ? 'Edit' : 'Add'} serial port
					</span>
					<button
						type="button"
						class="text-surface-content/60 hover:text-surface-content text-sm"
						onclick={closeEditPanel}
						aria-label="Close serial port editor"
					>
						Close
					</button>
				</header>

				<div class="flex-1 space-y-4 overflow-y-auto p-4">
					<TextField label="Configuration Name" bind:value={formData.name} required />

					<!-- Port path: SelectField for detected devices, with a
					     toggle to a free-text TextField for non-enumerated
					     paths (HAT pins, custom drivers, etc). -->
					<div class="space-y-2">
						{#if manualPortEntry}
							<TextField
								label="Port Path"
								bind:value={formData.port_path}
								placeholder="/dev/ttyUSB0"
							/>
						{:else}
							<!-- value + on:change instead of bind:value: SelectField's
							     internal setter throws "id is not defined" when bound
							     to a deep field on a plain `let` object in svelte 5
							     legacy mode.  Reassigning formData triggers reactivity. -->
							<SelectField
								label="Port Path"
								value={formData.port_path}
								on:change={(e) => {
									formData = {
										...formData,
										port_path: (e as CustomEvent).detail.value
									};
								}}
								options={portPathOptions}
								clearable={false}
								placeholder={portPathOptions.length === 0
									? 'No ports detected — Rescan or enter manually'
									: 'Select a detected port'}
							/>
						{/if}
						<div class="flex flex-wrap items-center gap-2">
							<Button
								on:click={rescanDevices}
								variant="outline"
								size="sm"
								disabled={rescanning}
								title="Re-scan for serial devices (use after plugging in a USB adapter)"
							>
								{rescanning ? 'Scanning...' : 'Rescan'}
							</Button>
							<Button
								on:click={() => (manualPortEntry = !manualPortEntry)}
								variant="outline"
								size="sm"
							>
								{manualPortEntry ? 'Pick from detected ports' : 'Enter path manually'}
							</Button>
							<p class="text-surface-content/60 text-xs">
								{portPathOptions.length} detected
							</p>
						</div>
					</div>

					<Field label="Baud Rate" let:id>
						<select
							{id}
							bind:value={formData.baud_rate}
							class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
						>
							{#each baudRateOptions as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
					</Field>

					<SelectField
						label="Sensor Model"
						value={formData.sensor_model}
						on:change={(e) => {
							formData = {
								...formData,
								sensor_model: (e as CustomEvent).detail.value
							};
						}}
						options={sensorModelOptions}
						clearable={false}
					/>

					<div class="grid grid-cols-3 gap-4">
						<Field label="Data Bits" let:id>
							<select
								{id}
								bind:value={formData.data_bits}
								class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
							>
								{#each dataBitsOptions as opt (opt.value)}
									<option value={opt.value}>{opt.label}</option>
								{/each}
							</select>
						</Field>

						<Field label="Stop Bits" let:id>
							<select
								{id}
								bind:value={formData.stop_bits}
								class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
							>
								{#each stopBitsOptions as opt (opt.value)}
									<option value={opt.value}>{opt.label}</option>
								{/each}
							</select>
						</Field>

						<Field label="Parity" let:id>
							<select
								{id}
								bind:value={formData.parity}
								class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
							>
								{#each parityOptions as opt (opt.value)}
									<option value={opt.value}>{opt.label}</option>
								{/each}
							</select>
						</Field>
					</div>

					<TextField label="Description" bind:value={formData.description} multiline rows={3} />

					<Field label="Enabled" let:id>
						<Checkbox {id} bind:checked={formData.enabled}>Enable</Checkbox>
					</Field>
				</div>

				<footer class="flex gap-2 border-t p-4">
					<Button on:click={handleSerialTest} variant="outline" disabled={testing}>
						{testing ? 'Testing...' : 'Test connection'}
					</Button>
					<div class="flex-1"></div>
					<Button on:click={closeEditPanel} variant="outline">Cancel</Button>
					<Button on:click={handleSerialSave} variant="fill" color="primary">Save</Button>
				</footer>
			</aside>
		{/if}
	</div>
</main>

<!-- Delete Confirmation (modal — destructive) -->
<Dialog bind:open={showDeleteDialog} class="max-w-md">
	<div class="space-y-4 p-6">
		<h2 class="text-xl font-semibold">Delete Configuration</h2>
		<p>
			Are you sure you want to delete the configuration "{deletingConfig?.name}"? This action cannot
			be undone.
		</p>
		<div class="flex gap-2 pt-4">
			<Button on:click={() => (showDeleteDialog = false)} variant="outline">Cancel</Button>
			<Button on:click={handleSerialDelete} variant="fill" color="danger">Delete</Button>
		</div>
	</div>
</Dialog>

<!-- Test Result (modal — one-shot read-only) -->
<Dialog bind:open={showTestResultDialog} class="max-w-2xl">
	<div class="space-y-4 p-6">
		<h2 class="text-xl font-semibold">Serial Port Test Results</h2>

		{#if testResult}
			<div class="space-y-3">
				<div
					class="rounded-lg p-4 {testResult.success
						? 'bg-success-50 text-success-900'
						: 'bg-danger-50 text-danger-900'}"
				>
					<p class="font-semibold">
						{testResult.success ? '✓ Success' : '✗ Failed'}
					</p>
					<p class="text-sm">{testResult.message}</p>
				</div>

				<div class="grid grid-cols-2 gap-4 text-sm">
					<div>
						<span class="font-semibold">Port:</span>
						{testResult.port_path}
					</div>
					<div>
						<span class="font-semibold">Baud Rate:</span>
						{testResult.baud_rate}
					</div>
					<div>
						<span class="font-semibold">Duration:</span>
						{testResult.test_duration_ms}ms
					</div>
					{#if testResult.bytes_received}
						<div>
							<span class="font-semibold">Bytes Received:</span>
							{testResult.bytes_received}
						</div>
					{/if}
				</div>

				{#if testResult.suggestion}
					<div class="bg-warning-50 text-warning-900 rounded-lg p-4">
						<p class="font-semibold">Suggestion:</p>
						<p class="text-sm">{testResult.suggestion}</p>
					</div>
				{/if}

				{#if testResult.sample_data}
					<div>
						<p class="mb-2 text-sm font-semibold">Sample Data:</p>
						<pre
							class="text-surface-content bg-surface-100 overflow-auto rounded-lg p-3 text-xs">{testResult.sample_data}</pre>
					</div>
				{/if}

				{#if testResult.raw_responses && testResult.raw_responses.length > 0}
					<div>
						<p class="mb-2 text-sm font-semibold">Raw Responses:</p>
						<div class="space-y-2">
							{#each testResult.raw_responses as resp, idx (idx)}
								<div class="bg-surface-100 rounded-lg p-3">
									<p class="text-xs font-semibold">Command: {resp.command}</p>
									<pre class="mt-1 overflow-auto text-xs">{resp.response}</pre>
									<p class="text-surface-content/70 mt-1 text-xs">
										{resp.is_json ? 'JSON Response' : 'Plain Text Response'}
									</p>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}

		<div class="flex justify-end pt-4">
			<Button on:click={() => (showTestResultDialog = false)} variant="fill">Close</Button>
		</div>
	</div>
</Dialog>
