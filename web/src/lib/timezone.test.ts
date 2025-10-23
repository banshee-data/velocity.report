import {
  AVAILABLE_TIMEZONES,
  getDisplayTimezone,
  getStoredTimezone,
  getTimezoneLabel,
  setStoredTimezone,
  type Timezone
} from './timezone';

describe('timezone', () => {
  describe('AVAILABLE_TIMEZONES', () => {
    it('should contain 56 timezone options', () => {
      expect(AVAILABLE_TIMEZONES).toHaveLength(56);
    });

    it('should have correct structure for each timezone', () => {
      AVAILABLE_TIMEZONES.forEach((tz) => {
        expect(tz).toHaveProperty('value');
        expect(tz).toHaveProperty('label');
        expect(typeof tz.value).toBe('string');
        expect(typeof tz.label).toBe('string');
      });
    });

    it('should include key timezones', () => {
      const values = AVAILABLE_TIMEZONES.map((tz) => tz.value);
      expect(values).toContain('America/New_York');
      expect(values).toContain('Europe/London');
      expect(values).toContain('Asia/Singapore');
      expect(values).toContain('Pacific/Auckland');
    });

    it('should include flag emojis in labels', () => {
      const singaporeLabel = AVAILABLE_TIMEZONES.find((tz) => tz.value === 'Asia/Singapore')?.label;
      expect(singaporeLabel).toContain('🇸🇬');
    });
  });

  describe('getStoredTimezone', () => {
    it('should return stored timezone from localStorage', () => {
      window.localStorage.setItem('display-timezone', 'Europe/London');
      expect(getStoredTimezone()).toBe('Europe/London');
    });

    it('should return null when no timezone is stored', () => {
      expect(getStoredTimezone()).toBeNull();
    });

    it('should return null when window is undefined (SSR)', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(getStoredTimezone()).toBeNull();
      global.window = originalWindow;
    });
  });

  describe('setStoredTimezone', () => {
    it('should store timezone in localStorage', () => {
      setStoredTimezone('America/New_York');
      expect(window.localStorage.getItem('display-timezone')).toBe('America/New_York');
    });

    it('should update existing stored timezone', () => {
      setStoredTimezone('Asia/Singapore');
      expect(window.localStorage.getItem('display-timezone')).toBe('Asia/Singapore');
      setStoredTimezone('Pacific/Auckland');
      expect(window.localStorage.getItem('display-timezone')).toBe('Pacific/Auckland');
    });

    it('should handle SSR gracefully when window is undefined', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(() => setStoredTimezone('UTC')).not.toThrow();
      global.window = originalWindow;
    });
  });

  describe('getDisplayTimezone', () => {
    it('should return stored timezone when available', () => {
      window.localStorage.setItem('display-timezone', 'Asia/Seoul');
      expect(getDisplayTimezone()).toBe('Asia/Seoul');
    });

    it('should return default timezone (UTC) when no timezone is stored', () => {
      expect(getDisplayTimezone()).toBe('UTC');
    });

    it('should return default timezone when window is undefined (SSR)', () => {
      const originalWindow = global.window;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      delete (global as any).window;
      expect(getDisplayTimezone()).toBe('UTC');
      global.window = originalWindow;
    });
  });

  describe('getTimezoneLabel', () => {
    it('should return correct label for UTC', () => {
      expect(getTimezoneLabel('UTC')).toBe('🌍 UTC (UTC)');
    });

    it('should return correct label for America/New_York', () => {
      expect(getTimezoneLabel('America/New_York')).toBe('🇺🇸 New York (EST/EDT)');
    });

    it('should return correct label for Asia/Singapore', () => {
      expect(getTimezoneLabel('Asia/Singapore')).toBe('🇸🇬 Singapore (SGT)');
    });

    it('should return correct label for Asia/Seoul', () => {
      expect(getTimezoneLabel('Asia/Seoul')).toBe('🇰🇷 Seoul (KST)');
    });

    it('should return correct label for Pacific/Bougainville', () => {
      expect(getTimezoneLabel('Pacific/Bougainville')).toBe('🇵🇬 Bougainville (BST)');
    });

    it('should return correct label for Europe/Athens', () => {
      expect(getTimezoneLabel('Europe/Athens')).toBe('🇬🇷 Athens (EET/EEST)');
    });

    it('should return correct label for Africa/Lagos', () => {
      expect(getTimezoneLabel('Africa/Lagos')).toBe('🇳🇬 Lagos (WAT)');
    });

    it('should return correct label for Africa/Abidjan', () => {
      expect(getTimezoneLabel('Africa/Abidjan')).toBe('🇨🇮 Abidjan (GMT)');
    });

    it('should return correct label for America/Santiago', () => {
      expect(getTimezoneLabel('America/Santiago')).toBe('🇨🇱 Santiago (CLT/CLST)');
    });

    it('should return correct label for Atlantic/South_Georgia', () => {
      expect(getTimezoneLabel('Atlantic/South_Georgia')).toBe('🇬🇸 South Georgia (GST)');
    });

    it('should handle all timezones in AVAILABLE_TIMEZONES', () => {
      AVAILABLE_TIMEZONES.forEach((tz) => {
        const label = getTimezoneLabel(tz.value as Timezone);
        expect(label).toBe(tz.label);
        expect(label.length).toBeGreaterThan(0);
      });
    });
  });
});
