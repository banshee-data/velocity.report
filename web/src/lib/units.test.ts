import {
  AVAILABLE_UNITS,
  getDisplayUnits,
  getStoredUnits,
  getUnitLabel,
  setStoredUnits,
  type Unit
} from './units';

describe('units', () => {
  describe('AVAILABLE_UNITS', () => {
    it('should contain all unit options', () => {
      expect(AVAILABLE_UNITS).toEqual([
        { value: 'mps', label: 'm/s' },
        { value: 'mph', label: 'mph' },
        { value: 'kmph', label: 'km/h' }
      ]);
    });
  });

  describe('getStoredUnits', () => {
    it('should return stored unit from localStorage', () => {
      window.localStorage.setItem('display-units', 'mph');
      expect(getStoredUnits()).toBe('mph');
    });

    it('should return null when no unit is stored', () => {
      expect(getStoredUnits()).toBeNull();
    });

    it('should return null when window is undefined (SSR)', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(getStoredUnits()).toBeNull();
      global.window = originalWindow;
    });
  });

  describe('setStoredUnits', () => {
    it('should store unit in localStorage', () => {
      setStoredUnits('kmph');
      expect(window.localStorage.getItem('display-units')).toBe('kmph');
    });

    it('should update existing stored unit', () => {
      setStoredUnits('mps');
      expect(window.localStorage.getItem('display-units')).toBe('mps');
      setStoredUnits('mph');
      expect(window.localStorage.getItem('display-units')).toBe('mph');
    });

    it('should handle SSR gracefully when window is undefined', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(() => setStoredUnits('mps')).not.toThrow();
      global.window = originalWindow;
    });
  });

  describe('getDisplayUnits', () => {
    it('should return stored unit when available', () => {
      window.localStorage.setItem('display-units', 'mph');
      expect(getDisplayUnits()).toBe('mph');
    });

    it('should return default unit (mps) when no unit is stored', () => {
      expect(getDisplayUnits()).toBe('mps');
    });

    it('should return default unit when window is undefined (SSR)', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(getDisplayUnits()).toBe('mps');
      global.window = originalWindow;
    });
  });

  describe('getUnitLabel', () => {
    it('should return correct label for mps', () => {
      expect(getUnitLabel('mps')).toBe('m/s');
    });

    it('should return correct label for mph', () => {
      expect(getUnitLabel('mph')).toBe('mph');
    });

    it('should return correct label for kmph', () => {
      expect(getUnitLabel('kmph')).toBe('km/h');
    });

    it('should return correct label for kph (alias)', () => {
      expect(getUnitLabel('kph')).toBe('km/h');
    });

    it('should handle all units in AVAILABLE_UNITS', () => {
      AVAILABLE_UNITS.forEach((unit) => {
        expect(getUnitLabel(unit.value as Unit)).toBe(unit.label);
      });
    });
  });
});
