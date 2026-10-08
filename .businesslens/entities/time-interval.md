---
references:
  - kind: code
    role: implementation
    target: timeinterval/timeinterval.go#TimeInterval
---

# Time interval

A named set of recurring periods, such as office hours or weekends, that
routes use to mute notifications or to send them only at certain times. A
time is inside the interval when it falls in any of its periods, and inside a
period when every field the period sets matches.

## Information kept

- **Name** — the unique name routes refer to
- **Times** — ranges of the day, start inclusive and end exclusive
- **Weekdays** — days or ranges of days of the week
- **Days of month** — days or ranges of days, negative values counting back from the end of the month
- **Months** — months or ranges of months
- **Years** — years or ranges of years
- **Location** — the time zone the periods are read in
