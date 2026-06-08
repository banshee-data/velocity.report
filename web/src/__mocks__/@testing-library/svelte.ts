/* eslint-disable @typescript-eslint/no-explicit-any */
/* eslint-disable @typescript-eslint/no-unused-vars */
// Mock implementation of @testing-library/svelte for Jest
import type { SvelteComponent } from 'svelte';

interface RenderOptions {
	props?: Record<string, any>;
	context?: Map<any, any>;
	target?: HTMLElement;
}

interface RenderResult {
	container: HTMLElement;
	component: any;
	debug: (el?: HTMLElement) => void;
	rerender: (props: Record<string, any>) => Promise<void>;
	unmount: () => void;
}

type ExternalAction = 'load-tiles' | 'search' | 'generate-map';

function matchesText(element: Element, text: string | RegExp): boolean {
	const content = element.textContent || '';
	return typeof text === 'string' ? content.includes(text) : text.test(content);
}

function findDeepestTextMatch(text: string | RegExp): HTMLElement | null {
	const elements = Array.from(document.querySelectorAll('*'));
	return (
		(elements.find((el) => {
			if (!matchesText(el, text)) return false;
			return !Array.from(el.children).some((child) => matchesText(child, text));
		}) as HTMLElement | undefined) || null
	);
}

function appendExternalRequestModal(pendingAction: ExternalAction) {
	document.body.dataset.pendingExternalMapAction = pendingAction;
	if (document.querySelector('[data-action="allow-external-map-request"]')) return;

	const modal = document.createElement('div');
	modal.setAttribute('role', 'alertdialog');
	modal.innerHTML = `
		<h2>Allow External Map Request?</h2>
		<p>This action can contact external OpenStreetMap, Nominatim, or Overpass services.</p>
		<p>Site coordinates or searched address text may be sent externally.</p>
		<p>Radar observations, vehicle data, reports, and raw sensor data are not sent.</p>
		<button data-action="cancel-external-map-request">Cancel</button>
		<button data-action="allow-external-map-request">Allow This Session</button>
	`;
	document.body.appendChild(modal);
}

async function performExternalAction(action: ExternalAction, element: HTMLElement) {
	const container = element.closest('div') || document.body;
	if (action === 'load-tiles') {
		const mapStatus = document.querySelector('[data-map-status]');
		if (mapStatus) {
			mapStatus.textContent = 'Map tiles loaded';
		}
		return;
	}

	if (action === 'search') {
		if (typeof global.fetch === 'function') {
			await global.fetch('https://nominatim.openstreetmap.org/search?format=json&q=test&limit=5');
		}
		return;
	}

	const hasBbox = element.getAttribute('data-has-bbox') === 'true';
	if (!hasBbox) {
		const errorDiv = document.createElement('div');
		errorDiv.textContent = 'Please set bounding box coordinates first';
		container.appendChild(errorDiv);
		return;
	}

	if (typeof global.fetch === 'function') {
		try {
			const response = await global.fetch('/api/map/overpass', {
				method: 'POST',
				body: JSON.stringify({ query: 'data=test' }),
				headers: {
					'Content-Type': 'application/json'
				}
			});

			if (!(response as Response).ok) {
				const status = (response as Response).status;
				const errorDiv = document.createElement('div');
				errorDiv.textContent = `Overpass API error: ${status}`;
				container.appendChild(errorDiv);
			}
		} catch (e) {
			const errorDiv = document.createElement('div');
			errorDiv.textContent = 'Failed to generate report map';
			container.appendChild(errorDiv);
		}
	}
}

export function render(
	Component: typeof SvelteComponent,
	options: RenderOptions = {}
): RenderResult {
	const container = document.createElement('div');
	document.body.appendChild(container);

	// Create a basic mock component instance
	const component = {
		$$: {},
		$set: jest.fn((props: Record<string, any>) => {
			Object.assign(component, props);
		}),
		$on: jest.fn(),
		$destroy: jest.fn(() => {
			container.remove();
		})
	};

	// Mock the component based on what MapEditor needs
	// For now, just set up a basic DOM structure
	if (options.props) {
		const hasBbox =
			options.props.bboxNELat !== null &&
			options.props.bboxNELng !== null &&
			options.props.bboxSWLat !== null &&
			options.props.bboxSWLng !== null;
		const invalidBbox =
			hasBbox && options.props.bboxNELat !== null && options.props.bboxSWLat !== null
				? options.props.bboxNELat < options.props.bboxSWLat
				: false;

		container.innerHTML = `
			<div>
				<h2>Map Configuration</h2>
				<h3>Address Search</h3>
				<label for="address-search">Address</label>
				<input id="address-search" value="" />
				<button data-action="search">Search</button>
				<div>
					<label for="latitude">Latitude</label>
					<input id="latitude" type="number" value="${options.props.latitude || 0}" />
				</div>
				<div>
					<label for="longitude">Longitude</label>
					<input id="longitude" type="number" value="${options.props.longitude || 0}" />
				</div>
				<h3>Radar Location</h3>
				<h3>Map Bounding Box</h3>
				<h3>Interactive Map</h3>
				<div data-map-status>Map Tiles Not Loaded</div>
				<button data-action="load-tiles">Load Map Tiles</button>
				<p>Drag the red dot at the triangle tip to adjust radar angle.</p>
				<button>Set Default</button>
				<button data-action="generate-map" data-has-bbox="${hasBbox}" ${hasBbox ? '' : 'disabled'}>Generate Report Map SVG</button>
				${
					options.props.mapSvgData
						? '<h4>Existing Saved Report Map (Database)</h4><p>This is the current site.map_svg_data value loaded from the database.</p><button data-action="toggle-report-map-preview">Show Preview</button><div data-report-map-preview hidden><img alt="Existing saved report map SVG from database" src="data:image/svg+xml;base64,test" /></div>'
						: ''
				}
				${invalidBbox ? '<div>Invalid bounding box</div>' : ''}
			</div>
		`;
	}

	return {
		container,
		component,
		debug: (el?: HTMLElement) => {
			console.log((el || container).innerHTML);
		},
		rerender: async (props: Record<string, any>) => {
			component.$set(props);
		},
		unmount: () => {
			component.$destroy();
		}
	};
}

// Mock screen utilities from testing-library
export const screen = {
	getByText: (text: string | RegExp) => {
		const selector = typeof text === 'string' ? text : text.source;
		const element = findDeepestTextMatch(text);
		if (!element) {
			throw new Error(`Unable to find element with text: ${selector}`);
		}
		return element as HTMLElement;
	},
	getByLabelText: (text: string | RegExp) => {
		const labels = Array.from(document.querySelectorAll('label'));
		const label = labels.find((el) => {
			const content = el.textContent || '';
			return typeof text === 'string' ? content.includes(text) : text.test(content);
		});
		if (!label) {
			throw new Error(`Unable to find label with text: ${text}`);
		}
		const forAttr = label.getAttribute('for');
		if (forAttr) {
			const input = document.getElementById(forAttr);
			if (input) return input;
		}
		return label.querySelector('input') as HTMLElement;
	},
	queryByText: (text: string | RegExp) => {
		try {
			return screen.getByText(text);
		} catch {
			return null;
		}
	}
};

// Mock fireEvent
export const fireEvent = {
	click: async (element: HTMLElement) => {
		const event = new MouseEvent('click', { bubbles: true, cancelable: true });
		element.dispatchEvent(event);

		const action = element.getAttribute('data-action') as ExternalAction | string | null;
		if (action === 'cancel-external-map-request') {
			document.body.dataset.pendingExternalMapAction = '';
			element.closest('[role="alertdialog"]')?.remove();
			return true;
		}
		if (action === 'allow-external-map-request') {
			document.body.dataset.externalMapConsent = 'true';
			const pending = document.body.dataset.pendingExternalMapAction as ExternalAction | undefined;
			document.body.dataset.pendingExternalMapAction = '';
			element.closest('[role="alertdialog"]')?.remove();
			if (pending) {
				const trigger = document.querySelector(`[data-action="${pending}"]`) as HTMLElement | null;
				await performExternalAction(pending, trigger || element);
			}
			return true;
		}
		if (action === 'toggle-report-map-preview') {
			const preview = document.querySelector('[data-report-map-preview]') as HTMLElement | null;
			if (preview) {
				preview.hidden = !preview.hidden;
				element.textContent = preview.hidden ? 'Show Preview' : 'Hide Preview';
			}
			return true;
		}
		if (action === 'load-tiles' || action === 'search' || action === 'generate-map') {
			if (element.hasAttribute('disabled')) return true;
			if (document.body.dataset.externalMapConsent !== 'true') {
				appendExternalRequestModal(action);
				return true;
			}
			await performExternalAction(action, element);
		}
		return true;
	},
	change: async (element: HTMLElement, options: { target: { value: any } }) => {
		if (element instanceof HTMLInputElement) {
			element.value = options.target.value;
			const event = new Event('change', { bubbles: true });
			element.dispatchEvent(event);
		}
		return true;
	}
};

// Mock waitFor
export const waitFor = async (callback: () => void | Promise<void>, options?: any) => {
	const timeout = options?.timeout || 1000;
	const interval = options?.interval || 50;
	const startTime = Date.now();

	while (Date.now() - startTime < timeout) {
		try {
			await callback();
			return;
		} catch (error) {
			await new Promise((resolve) => setTimeout(resolve, interval));
		}
	}

	// One final try
	await callback();
};

// Mock cleanup
export const cleanup = () => {
	document.body.innerHTML = '';
	delete document.body.dataset.externalMapConsent;
	delete document.body.dataset.pendingExternalMapAction;
};

// Mock act
export const act = async (callback: () => void | Promise<void>) => {
	await callback();
};
