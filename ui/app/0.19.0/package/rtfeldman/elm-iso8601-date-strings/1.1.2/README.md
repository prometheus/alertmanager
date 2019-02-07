# elm-iso8601

Convert [ISO-8601 date strings](https://en.wikipedia.org/wiki/ISO_8601) into [POSIX times](https://package.elm-lang.org/packages/elm/time/latest/Time#Posix).

This package takes the view that it is a mistake to use ISO-8601 strings as a
data transfer format. Nevertheless, third-party endpoints may use them,
so even if we'd rather avoid them, sometimes they may be unavoidable.

The only design goal of this package is to "correct the mistake" by translating
ISO-8601 strings to and from `Time.Posix` values. If it encounters a UTC offset
in the string, it normalizes and discards it such that the resulting `Time.Posix`
value is in UTC no matter what.

That's all this package does, and all it aims to do!

## Why are integers better?

Integer milliseconds since the epoch is a better choice for data transfer because
ISO-8601 strings can potentially include a UTC offset.

This is much worse than not having the possibility of including a UTC offset. Consider:

* UTC offsets are not time zones. This does not and cannot tell us the time zone in which the date was recorded. So what are we supposed to do with this information?
* Users typically want dates formatted according to their local time zone. What if the provided UTC offset is different from the current user's time zone? What are we supposed to do with it then?
* Despite it being useless (or worse, a source of bugs), the UTC offset creates a larger payload to transfer.

Even without the UTC offset, an ISO-8601 string is still much larger to transfer
than an integer. The usual argument in favor of ISO-8601 strings is that they
are more human-readable than integers, but making data readable for humans is
a better job for developer tools than data transfer formats between machines.

