import { get } from 'svelte/store';
import * as unitsModule from '../units';
import { displayUnits, initializeUnits, updateUnits } from './units';

// Mock the units module
jest.mock('../units');

describe('units store', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    // Reset store to default value
    displayUnits.set('mph');
  });

  describe('displayUnits store', () => {
    it('should initialize with default value', () => {
      const value = get(displayUnits);
      expect(value).toBe('mph');
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

      expect(currentValue).toBe('mph');

      displayUnits.set('mph');
      expect(currentValue).toBe('mph');

      unsubscribe();
    });
  });

  describe('initializeUnits', () => {
    it('should use stored units when available', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('mph');

      initializeUnits('mps');

      expect(unitsModule.getDisplayUnits).toHaveBeenCalledWith('mps');
      expect(get(displayUnits)).toBe('mph');
    });

    it('should use default units when no stored value', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('mps');

      initializeUnits('mps');

      expect(get(displayUnits)).toBe('mps');
    });

    it('should use server-provided units when specified', () => {
      (unitsModule.getDisplayUnits as jest.Mock).mockReturnValue('kmph');

      initializeUnits('kmph');

      expect(unitsModule.getDisplayUnits).toHaveBeenCalledWith('kmph');
      const storedValue = get(displayUnits);
      expect(storedValue).toBe('kmph');
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
