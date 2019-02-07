# Time

To work with time successfully in programming, we need three different concepts:

- **Human Time** &mdash; This is what you see on clocks (8am) or on calendars (May 3rd). Great! But if my phone call is at 8am in Boston, what time is it for my friend in Vancouver? If it is at 8am in Tokyo, is that even the same day in New York? (No!) So between [time zones][tz] based on ever-changing political boundaries and inconsistent use of [daylight saving time][dst], human time should basically never be stored in your `Model` or database! It is only for display!

- **POSIX Time** &mdash; With POSIX time, it does not matter where you live or what time of year it is. It is just the number of seconds elapsed since some arbitrary moment (in 1970). Everywhere you go on Earth, POSIX time is the same.

- **Time Zones** &mdash; A “time zone” is a bunch of data that allows you to turn POSIX time into human time. This is _not_ just `UTC-7` or `UTC+3` though! Time zones are way more complicated than a simple offset! Every time [Florida switches to DST forever][florida] or [Samoa switches from UTC-11 to UTC+13][samoa], some poor soul adds a note to the [IANA time zone database][iana]. That database is loaded onto every computer, and between POSIX time and all the corner cases in the database, we can figure out human times!

[tz]: https://en.wikipedia.org/wiki/Time_zone
[dst]: https://en.wikipedia.org/wiki/Daylight_saving_time
[iana]: https://en.wikipedia.org/wiki/IANA_time_zone_database
[samoa]: https://en.wikipedia.org/wiki/Time_in_Samoa
[florida]: https://www.npr.org/sections/thetwo-way/2018/03/08/591925587/

So to show a human being a time, you must always know **the POSIX time** and **their time zone**. That is it. So all that “human time” stuff is for your `view` function, not your `Model`.


## Example

To figure out a human time, you need to ask two questions: (1) what POSIX time is it? and (2) what time zone am I in? Once you have that, you can decide how to show it on screen:

```elm
import Time exposing (utc, toHour, toMinute, toSecond)

toUtcString : Time.Posix -> String
toUtcString time =
  String.fromInt (toHour utc time)
  ++ ":" ++
  String.fromInt (toMinute utc time)
  ++ ":" ++
  String.fromInt (toSecond utc time)
  ++ " (UTC)"
```

Notice that we provide the `utc` time zone to `toHour` and `toMinute`!

Go [here](https://elm-lang.org/examples/time) for a little example application that uses time. It can help you get everything hooked up in practice!


## Recurring Events

A lot of programmers need to handle recurring events. This meeting repeats every Monday. This event is the first Wednesday of each month. And there are always exceptions where a recurring event gets moved! **Using human time does not solve this!**

To properly handle recurring events, you need to create a custom `type` for your particular problem. Say you want to model a weekly event:

```elm
import Time

type alias WeeklyEvent =
  { weekday : Time.Weekday                  -- which day is it on
  , hour : Int                              -- at what hour?
  , zone : Time.Zone                        -- what time zone is that hour in?
  , start : Time.Posix                      -- when was the first event?
  , exceptions : List (Int, Maybe Event)    -- are there any skips?
  }
```

The first two fields (`weekday` and `hour`) are the most straight forward. You gotta know what day and what time! But that is not enough information for people in different time zones. Tom created the recurring event for hour 16, but how do I show that in Tokyo or New York? Or even in Tom’s location?! The `zone` lets us pin `weekday` and `hour` to a specific POSIX time, so we can show it elsewhere.

Great! But what about shifting the meeting by one day for a holiday? Well, if you define a `start` time, you can store `exceptions` as offsets from the first ever event. So if _only_ the third event was cancelled, you could store `[ (3, Nothing) ]` which would say “ignore the third event, and do not replace it with some other specific event.”

### Implications

Now the particular kinds of recurring events _you_ need are specific to _your_ application. Weekly? Monthly? Always has start and end of range? Allows exceptions? I am not convinced that a generic design is possible for all scenarios, but maybe with further exploration, we will find that it is.

**So if you need recurring events, you have to model them yourself.** There is no shortcut. Putting May 3rd in your `Model` is not gonna do it. It is a trap. Thinking in human time is always a trap!


## ISO 8601

[ISO 8601][8601] is not supported by this package because:

> The ISO 8601 format has lead to a great deal of confusion. Specifically because it gives the _illusion_ that it can handle time zones. It cannot! It allows you to specify an offset from UTC like `-05:00`, but is that a time in Quebec, Miami, Cuba, or Equador? Are they doing daylight saving time right now?
>
> Point is, **the only thing ISO 8601 is good for is representing a `Time.Posix`, but less memory efficient and more confusing.** So I recommend using `Time.posixToMillis` and `Time.millisToPosix` for any client/server communication you control.

That said, many endpoints use ISO 8601 for some reason, and it can therefore be quite useful in practice. I think the community should make some packages that define `fromIso8601 : String -> Maybe Time.Posix` in Elm. People can use `elm/parser` to make a fancy implementation, but maybe there is some faster or smaller implementation possible with `String.split` and such. Folks should experiment, and later on, we can revisit if any of them belong in this library.

[8601]: https://en.wikipedia.org/wiki/ISO_8601


## Future Plans

Right now this library gives basic `Posix` and `Zone` functions, but there are a couple important things it does not cover right now:

  1. How do I get _my_ time zone?
  2. How do I get another time zone by name?
  3. How do I display a time for a specific region? (e.g. `DD/MM/YYYY` vs `MM/DD/YYYY`)

I think points (2) and (3) should be explored by the community before we add anything here. Maybe we can have a package that hard codes the IANA time zone database? Maybe we can have a package that provides HTTP requests to ask for specific time zone data? Etc.

**Note:** If you make progress that potentially needs coordination with other developers, **talk to people**. Present your work on [discourse](https://discourse.elm-lang.org/) to learn what the next steps might be. Is the idea good? Does it need more work? Are there other things to consider? Etc. Just opening an issue like “I totally redid the API” is not how we run things here, so focus on making a strong case through normal packages and friendly communication! And on timelines, we try to make one _great_ choice ever (not a _different_ choice every month) so things will take longer than in the JS world.
