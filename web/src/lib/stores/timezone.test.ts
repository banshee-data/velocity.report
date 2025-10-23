import { get } from 'svelte/store';
import type { Timezone } from '../timezone';
import * as timezoneModule from '../timezone';
import { displayTimezone, initializeTimezone, updateTimezone } from './timezone';

// Mock the timezone module
jest.mock('../timezone');

describe('timezone store', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    // Reset store to default value
    displayTimezone.set('UTC');
  });

  describe('displayTimezone store', () => {
    it('should initialize with default value', () => {
      const value = get(displayTimezone);
      expect(value).toBe('UTC');
    });

    it('should be writable', () => {
      displayTimezone.set('America/New_York');
      expect(get(displayTimezone)).toBe('America/New_York');

      displayTimezone.set('Europe/Dublin');
      expect(get(displayTimezone)).toBe('Europe/Dublin');
    });

    it('should support subscription', () => {
      let currentValue = '';
      const unsubscribe = displayTimezone.subscribe((value) => {
        currentValue = value;
      });

      expect(currentValue).toBe('UTC');

      displayTimezone.set('Asia/Singapore');
      expect(currentValue).toBe('Asia/Singapore');

      unsubscribe();
    });
  });

  describe('initializeTimezone', () => {
    it('should use stored timezone when available', () => {
      (timezoneModule.getDisplayTimezone as jest.Mock).mockReturnValue('America/New_York');

      initializeTimezone('UTC');

      expect(timezoneModule.getDisplayTimezone).toHaveBeenCalledWith('UTC');
      expect(get(displayTimezone)).toBe('America/New_York');
    });

    it('should use default timezone when no stored value', () => {
      (timezoneModule.getDisplayTimezone as jest.Mock).mockReturnValue('UTC');

      initializeTimezone('UTC');

      expect(get(displayTimezone)).toBe('UTC');
    });

    it('should use server-provided timezone when specified', () => {
      (timezoneModule.getDisplayTimezone as jest.Mock).mockReturnValue('Europe/Dublin');

      initializeTimezone('Europe/Dublin');

      expect(timezoneModule.getDisplayTimezone).toHaveBeenCalledWith('Europe/Dublin');
      const storedValue = get(displayTimezone);
      expect(storedValue).toBe('Europe/Dublin');
    });

    it('should handle IANA timezone identifiers', () => {
      const timezones = [
        'America/New_York',
        'Europe/Dublin',
        'Asia/Singapore',
        'Asia/Seoul',
        'Pacific/Bougainville',
        'Europe/Athens'
      ];

      timezones.forEach((tz) => {
        (timezoneModule.getDisplayTimezone as jest.Mock).mockReturnValue(tz);
        initializeTimezone('UTC');
        expect(get(displayTimezone)).toBe(tz);
      });
    });
  });

  describe('updateTimezone', () => {
    it('should update store and persist to localStorage', () => {
      updateTimezone('America/Los_Angeles');

      expect(timezoneModule.setStoredTimezone).toHaveBeenCalledWith('America/Los_Angeles');
      expect(get(displayTimezone)).toBe('America/Los_Angeles');
    });

    it('should handle multiple updates', () => {
      updateTimezone('America/New_York');
      expect(get(displayTimezone)).toBe('America/New_York');

      updateTimezone('Europe/Dublin');
      expect(get(displayTimezone)).toBe('Europe/Dublin');

      updateTimezone('UTC');
      expect(get(displayTimezone)).toBe('UTC');

      expect(timezoneModule.setStoredTimezone).toHaveBeenCalledTimes(3);
    });

    it('should persist recently updated timezones', () => {
      const timezones: Timezone[] = [
        'Africa/Lagos',
        'Africa/Abidjan',
        'America/Santiago',
        'Atlantic/South_Georgia'
      ];

      timezones.forEach((tz) => {
        updateTimezone(tz);
        expect(timezoneModule.setStoredTimezone).toHaveBeenCalledWith(tz);
      });
    });
  });
});
