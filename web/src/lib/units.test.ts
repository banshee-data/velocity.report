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
        { value: 'mph', label: 'Miles per hour (mph)' },
        { value: 'kmph', label: 'Kilometers per hour (km/h)' },
        { value: 'mps', label: 'Meters per second (m/s)' }
      ]);
    });
  });

  describe('getStoredUnits', () => {
    it('should return stored unit from localStorage', () => {
      window.localStorage.setItem('velocity-report-units', 'mph');
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
      expect(window.localStorage.getItem('velocity-report-units')).toBe('kmph');
    });

    it('should update existing stored unit', () => {
      setStoredUnits('mps');
      expect(window.localStorage.getItem('velocity-report-units')).toBe('mps');
      setStoredUnits('mph');
      expect(window.localStorage.getItem('velocity-report-units')).toBe('mph');
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
      window.localStorage.setItem('velocity-report-units', 'mph');
      expect(getDisplayUnits('mps')).toBe('mph');
    });

    it('should return default unit (mps) when no unit is stored', () => {
      expect(getDisplayUnits('mps')).toBe('mps');
    });

    it('should return default unit when window is undefined (SSR)', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(getDisplayUnits('mps')).toBe('mps');
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
      // getUnitLabel returns short labels (m/s, mph, km/h)
      // but AVAILABLE_UNITS has verbose labels
      // So we just check that all units can be processed
      AVAILABLE_UNITS.forEach((unit) => {
        const label = getUnitLabel(unit.value as Unit);
        expect(label.length).toBeGreaterThan(0);
        expect(typeof label).toBe('string');
      });
    });

    it('should return the unit as-is for unknown units', () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      expect(getUnitLabel('unknown_unit' as any)).toBe('unknown_unit');
    });
  });
});
