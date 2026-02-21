import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';
import relativeTime from 'dayjs/plugin/relativeTime';
import utc from 'dayjs/plugin/utc';

dayjs.extend(utc);

dayjs.extend(duration);
dayjs.extend(relativeTime);

export const utcNow = () => dayjs().utc();

export const ISO8601 = 'YYYY-MM-DDTHH:mm:ssZ';

export function parseMantineDateTime(value: string): dayjs.Dayjs {
  // The Mantine DateTimePicker returns local time strings, so parse them as local and convert to UTC
  return dayjs(value).utc();
}

export function parseISO8601(value: string): dayjs.Dayjs {
  return dayjs.utc(value);
}

export function parseTime(timeText: string): dayjs.Dayjs {
  return dayjs.utc(timeText);
}

export function parseDuration(durationText: string): duration.Duration | null {
  if (durationText === '') {
    return null;
  }

  const durationRE = new RegExp(
    '^((?<year>[0-9]+)y)?((?<week>[0-9]+)w)?((?<day>[0-9]+)d)?((?<hour>[0-9]+)h)?((?<min>[0-9]+)m)?((?<sec>[0-9]+)s)?((?<ms>[0-9]+)ms)?$'
  );
  const matches = durationRE.exec(durationText);
  if (!matches || !matches.groups) {
    return null;
  }

  const years = matches.groups.year ? parseInt(matches.groups.year, 10) : 0;
  const weeks = matches.groups.week ? parseInt(matches.groups.week, 10) : 0;
  const days = matches.groups.day ? parseInt(matches.groups.day, 10) : 0;
  const hours = matches.groups.hour ? parseInt(matches.groups.hour, 10) : 0;
  const minutes = matches.groups.min ? parseInt(matches.groups.min, 10) : 0;
  const seconds = matches.groups.sec ? parseInt(matches.groups.sec, 10) : 0;
  const milliseconds = matches.groups.ms ? parseInt(matches.groups.ms, 10) : 0;

  return dayjs.duration({
    years,
    weeks,
    days,
    hours,
    minutes,
    seconds,
    milliseconds,
  });
}

export function formatDurationMs(ms: number): string {
  let dur = dayjs.duration(ms);
  let result = '';

  if (dur.years() > 0) {
    result += `${dur.years()}y`;
    dur = dur.subtract(dur.years(), 'years');
  }
  if (dur.weeks() > 0) {
    result += `${dur.weeks()}w`;
    dur = dur.subtract(dur.weeks(), 'weeks');
  }
  if (dur.days() > 0) {
    result += `${dur.days()}d`;
  }
  if (dur.hours() > 0) {
    result += `${dur.hours()}h`;
  }
  if (dur.minutes() > 0) {
    result += `${dur.minutes()}m`;
  }
  if (dur.seconds() > 0) {
    result += `${dur.seconds()}s`;
  }
  if (dur.milliseconds() > 0) {
    result += `${dur.milliseconds()}ms`;
  }
  return result || '0ms';
}
