# gotime
A go library for defining windows of time and validating points in time against those periods.

# How to Use
The main struct, the TimeInterval, is designed to be instantiated by a yaml configuration file:
```yaml
  # Last week, excluding Saturday, of the first quarter of the year during business hours from 2020 to 2025 and 2030-2035
- weekdays: ['monday:friday', 'sunday']
  months: ['january:march']
  days_of_month: ['-7:-1']
  years: ['2020:2025', '2030:2035']
  times:
    - start_time: '09:00'
      end_time: '17:00'
```
