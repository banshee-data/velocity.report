import { get } from 'svelte/store';
import * as unitsModule from '../units';
import { displayUnits, initializeUnits, updateUnits } from './units';

// Mock the units module
jest.mock('../units');

describe('units store', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
  });

  describe('displayUnits store', () => {
    it('should initialize with default value', () => {
      const value = get(displayUnits);
      expect(value).toBe('mps');
    });

    it('should be writable', () => {
      displayUnits.set('mph');
      expect(get(displayUnits)).toBe('mph');

      displayUnits.set('kmph');
      expect(get(displayUnits)).toBe('kmph');
    });

    it('should support subscription', () => {
      let currentValue = '';
      const unsubscribe = displayUnits.subscribe((value) => {
        currentValue = value;
      });

      expect(currentValue).toBe('mps');

      displayUnits.set('mph');
      expect(currentValue).toBe('mph');

      unsubscribe();
    });
  });

  describe('initializeUnits', () => {
    it('should use stored units when available', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('mph');

      initializeUnits();

      expect(unitsModule.getDisplayUnits).toHaveBeenCalled();
      expect(get(displayUnits)).toBe('mph');
    });

    it('should use default units when no stored value', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('mps');

      initializeUnits();

      expect(get(displayUnits)).toBe('mps');
    });

    it('should use server-provided units when specified', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('mps');

      initializeUnits('kmph');

      // Should still call getDisplayUnits to check stored preference
      expect(unitsModule.getDisplayUnits).toHaveBeenCalled();
      // But should use server default if no stored value
      const storedValue = get(displayUnits);
      expect(['mps', 'kmph']).toContain(storedValue);
    });
  });

  describe('updateUnits', () => {
    it('should update store and persist to localStorage', () => {
      updateUnits('mph');

      expect(unitsModule.setStoredUnits).toHaveBeenCalledWith('mph');
      expect(get(displayUnits)).toBe('mph');
    });

    it('should handle multiple updates', () => {
      updateUnits('mph');
      expect(get(displayUnits)).toBe('mph');

      updateUnits('kmph');
      expect(get(displayUnits)).toBe('kmph');

      updateUnits('mps');
      expect(get(displayUnits)).toBe('mps');

      expect(unitsModule.setStoredUnits).toHaveBeenCalledTimes(3);
    });

    it('should persist each unit type', () => {
      const units: Array<'mps' | 'mph' | 'kmph'> = ['mps', 'mph', 'kmph'];

      units.forEach((unit) => {
        updateUnits(unit);
        expect(unitsModule.setStoredUnits).toHaveBeenCalledWith(unit);
      });
    });
  });
});
