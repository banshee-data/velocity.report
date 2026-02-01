/* eslint-disable @typescript-eslint/no-explicit-any */
/* eslint-disable @typescript-eslint/no-unused-vars */
/* eslint-disable @typescript-eslint/no-unsafe-function-type */
// Generic mock for Svelte components
export default class MockSvelteComponent {
	$$prop_def: any;
	$$events_def: any;
	$$slot_def: any;

	constructor(options: any) {
		// Mock constructor
	}

	$set(props: any) {
		// Mock $set method
	}

	$on(event: string, handler: Function) {
		// Mock $on method
		return () => {}; // Return unsubscribe function
	}

	$destroy() {
		// Mock $destroy method
	}
}
