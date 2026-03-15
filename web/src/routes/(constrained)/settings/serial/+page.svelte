<script lang="ts">
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import {
		Button,
		Card,
		Checkbox,
		Dialog,
		Field,
		Header,
		Notification,
		TextField
	} from 'svelte-ux';
	import {
		createSerialConfig,
		deleteSerialConfig,
		getSensorModels,
		getSerialConfigs,
		getSerialDevices,
		testSerialPort,
		updateSerialConfig,
		type SensorModel,
		type SerialConfig,
		type SerialConfigRequest,
		type SerialDevice,
		type SerialTestResponse
	} from '$lib/api';

	// State
	let configs = $state<SerialConfig[]>([]);
	let sensorModels = $state<SensorModel[]>([]);
	let availableDevices = $state<SerialDevice[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');

	// Load all data on mount
	onMount(async () => {
		await loadData();
	});

	// Dialog states
	let showEditDialog = $state(false);
	let showDeleteDialog = $state(false);
	let showTestResultDialog = $state(false);

	// Edit form state
	let editingConfig = $state<SerialConfig | null>(null);
	let formData = $state<SerialConfigRequest>({
		name: '',
		port_path: '',
		baud_rate: 19200,
		data_bits: 8,
		stop_bits: 1,
		parity: 'N',
		enabled: true,
		description: '',
		sensor_model: 'ops243-a'
	});

	// Test result state
	let testResult = $state<SerialTestResponse | null>(null);
	let testing = $state(false);

	// Delete confirmation state
	let deletingConfig = $state<SerialConfig | null>(null);

	async function loadData() {
		try {
			loading = true;
			const [configsData, modelsData, devicesData] = await Promise.all([
				getSerialConfigs(),
				getSensorModels(),
				getSerialDevices()
			]);
			configs = configsData;
			sensorModels = modelsData;
			availableDevices = devicesData;

			// Build comprehensive port options including all devices and existing configs
			const uniquePortPaths = new SvelteSet<string>();
			devicesData.forEach((d) => uniquePortPaths.add(d.port_path));
			configsData.forEach((c) => uniquePortPaths.add(c.port_path));

			portPathOptions = Array.from(uniquePortPaths)
				.sort()
				.map((path) => ({
					value: path,
					label: path
				}));

			sensorModelOptions = sensorModels.map((model) => ({
				value: model.slug,
				label: model.display_name
			}));
		} catch (e) {
			console.error('Failed to load data:', e);
			showMessage('Failed to load configuration data', 'error');
		} finally {
			loading = false;
		}
	}

	function showMessage(msg: string, type: 'success' | 'error' | 'info' = 'info') {
		message = msg;
		messageType = type;
		setTimeout(() => {
			message = '';
		}, 5000);
	}

	function openCreateDialog() {
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

		showEditDialog = true;
	}

	function openEditDialog(config: SerialConfig) {
		editingConfig = config;
		formData = {
			name: config.name,
			port_path: config.port_path,
			baud_rate: config.baud_rate,
			data_bits: config.data_bits,
			stop_bits: config.stop_bits,
			parity: config.parity,
			enabled: config.enabled,
			description: config.description,
			sensor_model: config.sensor_model
		};

		showEditDialog = true;
	}

	function openDeleteDialog(config: SerialConfig) {
		deletingConfig = config;
		showDeleteDialog = true;
	}

	async function handleSave() {
		try {
			if (editingConfig) {
				await updateSerialConfig(editingConfig.id, formData);
				showMessage('Configuration updated successfully', 'success');
			} else {
				await createSerialConfig(formData);
				showMessage('Configuration created successfully', 'success');
			}
			showEditDialog = false;
			await loadData();
		} catch (e) {
			console.error('Failed to save config:', e);
			showMessage(`Failed to save configuration: ${e}`, 'error');
		}
	}

	async function handleDelete() {
		if (!deletingConfig) return;

		try {
			await deleteSerialConfig(deletingConfig.id);
			showMessage('Configuration deleted successfully', 'success');
			showDeleteDialog = false;
			deletingConfig = null;
			await loadData();
		} catch (e) {
			console.error('Failed to delete config:', e);
			showMessage(`Failed to delete configuration: ${e}`, 'error');
		}
	}

	async function handleTest() {
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
			showMessage(`Failed to test serial port: ${e}`, 'error');
		} finally {
			testing = false;
		}
	}

	// Option arrays
	const baudRates = [9600, 19200, 38400, 57600, 115200];
	const parityOptions = [
		{ value: 'N', label: 'None' },
		{ value: 'E', label: 'Even' },
		{ value: 'O', label: 'Odd' }
	];
	const dataBitsArray = [5, 6, 7, 8];
	const stopBitsArray = [1, 2];

	let portPathOptions = $state<{ value: string; label: string }[]>([]);
	const baudRateOptions = baudRates.map((rate) => ({ value: rate, label: rate.toString() }));
	const dataBitsOptions = dataBitsArray.map((n) => ({ value: n, label: n.toString() }));
	const stopBitsOptions = stopBitsArray.map((n) => ({ value: n, label: n.toString() }));
	let sensorModelOptions = $state<{ value: string; label: string }[]>([]);
</script>

<svelte:head>
	<title>Serial Configuration ⚙️ velocity.report</title>
	<meta name="description" content="Configure radar serial port settings" />
</svelte:head>

<div id="main-content" class="space-y-6 p-4">
	<Header
		title="Serial Configuration"
		subheading="Configure and test radar sensor serial port connections."
	/>

	{#if message}
		<Notification
			title={messageType === 'success' ? 'Success' : messageType === 'error' ? 'Error' : 'Info'}
			description={message}
			variant={messageType === 'error' ? 'fill' : 'default'}
			class={messageType === 'success'
				? 'bg-success-50 text-success-900 border-success-200'
				: messageType === 'error'
					? 'bg-danger-50 text-danger-900 border-danger-200'
					: 'bg-info-50 text-info-900 border-info-200'}
		/>
	{/if}

	{#if loading}
		<Card>
			<div class="p-4" role="status" aria-live="polite">
				<p>Loading serial configurations...</p>
			</div>
		</Card>
	{:else}
		<Card>
			<div class="space-y-4 p-4">
				<div class="flex items-center justify-between">
					<h2 class="text-lg font-semibold">Serial Port Configurations</h2>
					<Button on:click={openCreateDialog} variant="fill" color="primary">
						Add Serial Port
					</Button>
				</div>

				{#if configs.length === 0}
					<p class="text-surface-content/70">No serial configurations found.</p>
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
								{#each configs as row (row.id)}
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
												<Button on:click={() => openEditDialog(row)} size="sm" variant="outline">
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
		</Card>
	{/if}

	<!-- Edit/Create Dialog -->
	<Dialog bind:open={showEditDialog} class="max-w-2xl">
		<div class="space-y-4 p-6">
			<h2 class="text-xl font-semibold">
				{editingConfig ? 'Edit' : 'Create'} Serial Configuration
			</h2>

			<TextField label="Configuration Name" bind:value={formData.name} required />

			<Field label="Port Path" let:id>
				<select
					{id}
					bind:value={formData.port_path}
					class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
				>
					{#each portPathOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</Field>

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

			<Field label="Sensor Model" let:id>
				<select
					{id}
					bind:value={formData.sensor_model}
					class="border-surface-content/20 bg-surface-100 focus:border-primary focus:ring-primary/20 w-full rounded border px-3 py-2 text-sm focus:ring-2 focus:outline-none"
				>
					{#each sensorModelOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</Field>

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

			<div class="flex gap-2 pt-4">
				<Button on:click={handleTest} variant="outline" disabled={testing}>
					{testing ? 'Testing...' : 'Test Connection'}
				</Button>
				<div class="flex-1"></div>
				<Button on:click={() => (showEditDialog = false)} variant="outline">Cancel</Button>
				<Button on:click={handleSave} variant="fill" color="primary">Save</Button>
			</div>
		</div>
	</Dialog>

	<!-- Delete Confirmation Dialog -->
	<Dialog bind:open={showDeleteDialog} class="max-w-md">
		<div class="space-y-4 p-6">
			<h2 class="text-xl font-semibold">Delete Configuration</h2>
			<p>
				Are you sure you want to delete the configuration "{deletingConfig?.name}"? This action
				cannot be undone.
			</p>
			<div class="flex gap-2 pt-4">
				<Button on:click={() => (showDeleteDialog = false)} variant="outline">Cancel</Button>
				<Button on:click={handleDelete} variant="fill" color="danger">Delete</Button>
			</div>
		</div>
	</Dialog>

	<!-- Test Result Dialog -->
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
</div>
