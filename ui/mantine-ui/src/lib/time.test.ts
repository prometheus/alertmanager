import { formatDurationMs, parseDuration } from './time';

describe('parseDuration', () => {
  it('returns null for empty string', () => {
    const dur = parseDuration('');
    expect(dur).toBeNull();
  });

  it('parses seconds correctly', () => {
    const dur = parseDuration('45s');
    expect(dur).not.toBeNull();
    expect(dur!.asSeconds()).toBe(45);
  });

  it('parses minutes correctly', () => {
    const dur = parseDuration('2m');
    expect(dur).not.toBeNull();
    expect(dur!.asMinutes()).toBe(2);
  });

  it('parses hours correctly', () => {
    const dur = parseDuration('3h');
    expect(dur).not.toBeNull();
    expect(dur!.asHours()).toBe(3);
  });

  it('parses days correctly', () => {
    const dur = parseDuration('1d');
    expect(dur).not.toBeNull();
    expect(dur!.asDays()).toBe(1);
  });

  it('parses weeks correctly', () => {
    const dur = parseDuration('2w');
    expect(dur).not.toBeNull();
    expect(dur!.asWeeks()).toBe(2);
  });

  it('parses years correctly', () => {
    const dur = parseDuration('1y');
    expect(dur).not.toBeNull();
    expect(dur!.asYears()).toBe(1);
  });

  it('parses complex duration correctly', () => {
    const dur = parseDuration('1y2w3d4h5m6s7ms');
    expect(dur).not.toBeNull();
    expect(dur!.asMilliseconds()).toBe(
      1 * 365 * 24 * 60 * 60 * 1000 + // years
        2 * 7 * 24 * 60 * 60 * 1000 + // weeks
        3 * 24 * 60 * 60 * 1000 + // days
        4 * 60 * 60 * 1000 + // hours
        5 * 60 * 1000 + // minutes
        6 * 1000 + // seconds
        7 // milliseconds
    );
  });

  it('returns null for invalid format', () => {
    const dur = parseDuration('invalid123');
    expect(dur).toBeNull();
  });
});

describe('formatDurationMs', () => {
  it('formats milliseconds correctly', () => {
    expect(formatDurationMs(1500)).toBe('1s500ms');
  });

  it('formats seconds correctly', () => {
    expect(formatDurationMs(45000)).toBe('45s');
  });

  it('formats minutes correctly', () => {
    expect(formatDurationMs(120000)).toBe('2m');
  });

  it('formats hours correctly', () => {
    expect(formatDurationMs(1000 * 60 * 60 * 3)).toBe('3h');
  });

  it('formats days correctly', () => {
    expect(formatDurationMs(1000 * 60 * 60 * 24)).toBe('1d');
  });

  it('formats weeks correctly', () => {
    expect(formatDurationMs(1000 * 60 * 60 * 24 * 7 * 2)).toBe('2w');
  });

  it('formats years correctly', () => {
    expect(formatDurationMs(1000 * 60 * 60 * 24 * 365)).toBe('1y');
  });

  it('formats complex duration correctly', () => {
    const totalMs =
      1 * 365 * 24 * 60 * 60 * 1000 + // years
      2 * 7 * 24 * 60 * 60 * 1000 + // weeks
      3 * 24 * 60 * 60 * 1000 + // days
      4 * 60 * 60 * 1000 + // hours
      5 * 60 * 1000 + // minutes
      6 * 1000 + // seconds
      7; // milliseconds
    expect(formatDurationMs(totalMs)).toBe('1y2w3d4h5m6s7ms');
  });
});
