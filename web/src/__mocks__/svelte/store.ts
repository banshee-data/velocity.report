// Mock implementation of svelte/store for Jest tests
/* eslint-disable @typescript-eslint/no-unused-vars */
/* eslint-disable @typescript-eslint/no-explicit-any */

export interface Writable<T> {
  subscribe(run: (value: T) => void): () => void;
  set(value: T): void;
  update(updater: (value: T) => T): void;
}

export function writable<T>(value: T): Writable<T> {
  let subscribers: Array<(value: T) => void> = [];
  let currentValue = value;

  return {
    subscribe(run: (value: T) => void) {
      subscribers.push(run);
      run(currentValue);
      return () => {
        subscribers = subscribers.filter((s) => s !== run);
      };
    },
    set(newValue: T) {
      currentValue = newValue;
      subscribers.forEach((run) => run(currentValue));
    },
    update(updater: (value: T) => T) {
      currentValue = updater(currentValue);
      subscribers.forEach((run) => run(currentValue));
    }
  };
}

export function get<T>(store: Writable<T>): T {
  let value!: T;
  store.subscribe((v) => {
    value = v;
  })();
  return value;
}

// Minimal mock - we don't use these in our tests
export function readable<T>(
  value: T,
  _start?: (set: (value: T) => void) => () => void
): {
  subscribe(run: (value: T) => void): () => void;
} {
  const { subscribe } = writable(value);
  return { subscribe };
}

export function derived<S, T>(
  stores: any,
  fn: (values: S) => T,
  initial?: T
): {
  subscribe(run: (value: T) => void): () => void;
} {
  const single = !Array.isArray(stores);
  const storesArray = single ? [stores] : stores;
  const w = writable(initial as T);

  const sync = () => {
    const values = storesArray.map((store: any) => get(store));
    w.set(fn(single ? values[0] : (values as any)));
  };

  storesArray.forEach((store: any) => store.subscribe(sync));

  return {
    subscribe: w.subscribe
  };
}
