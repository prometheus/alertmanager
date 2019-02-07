effect module Time where { subscription = MySub } exposing
  ( Posix
  , now
  , every
  , posixToMillis
  , millisToPosix
  , Zone
  , utc
  , here
  , toYear
  , toMonth
  , toDay
  , toWeekday
  , toHour
  , toMinute
  , toSecond
  , toMillis
  , Month(..)
  , Weekday(..)
  , customZone
  , getZoneName
  , ZoneName(..)
  )


{-| Library for working with time and time zones.

# Time
@docs Posix, now, every, posixToMillis, millisToPosix

# Time Zones
@docs Zone, utc, here

# Human Times
@docs toYear, toMonth, toDay, toWeekday, toHour, toMinute, toSecond, toMillis

# Weeks and Months
@docs Weekday, Month

# For Package Authors
@docs customZone, getZoneName, ZoneName

-}


import Basics exposing (..)
import Dict
import Elm.Kernel.Time
import List exposing ((::))
import Maybe exposing (Maybe(..))
import Platform
import Platform.Sub exposing (Sub)
import Process
import String exposing (String)
import Task exposing (Task)



-- POSIX


{-| A computer representation of time. It is the same all over Earth, so if we
have a phone call or meeting at a certain POSIX time, there is no ambiguity.

It is very hard for humans to _read_ a POSIX time though, so we use functions
like [`toHour`](#toHour) and [`toMinute`](#toMinute) to `view` them.
-}
type Posix = Posix Int


{-| Get the POSIX time at the moment when this task is run.
-}
now : Task x Posix
now =
  Elm.Kernel.Time.now millisToPosix


{-| Turn a `Posix` time into the number of milliseconds since 1970 January 1
at 00:00:00 UTC. It was a Thursday.
-}
posixToMillis : Posix -> Int
posixToMillis (Posix millis) =
  millis


{-| Turn milliseconds into a `Posix` time.
-}
millisToPosix : Int -> Posix
millisToPosix =
  Posix



-- TIME ZONES


{-| Information about a particular time zone.

The [IANA Time Zone Database][iana] tracks things like UTC offsets and
daylight-saving rules so that you can turn a `Posix` time into local times
within a time zone.

See [`utc`](#utc), [`here`](#here), and [`Browser.Env`][env] to learn how to
obtain `Zone` values.

[iana]: https://www.iana.org/time-zones
[env]: /packages/elm/browser/latest/Browser#Env
-}
type Zone =
  Zone Int (List Era)


-- TODO: add this note back to `Zone` docs when it is true
--
-- Did you know that in California the times change from 3pm PST to 3pm PDT to
-- capture whether it is daylight-saving time? The database tracks those
-- abbreviation changes too. (Tons of time zones do that actually.)
--


{-| Currently the public API only needs:

- `start` is the beginning of this `Era` in "minutes since the Unix Epoch"
- `offset` is the UTC offset of this `Era` in minutes

But eventually, it will make sense to have `abbr : String` for `PST` vs `PDT`
-}
type alias Era =
  { start : Int
  , offset : Int
  }


{-| The time zone for Coordinated Universal Time ([UTC][])

The `utc` zone has no time adjustments. It never observes daylight-saving
time and it never shifts around based on political restructuring.

[UTC]: https://en.wikipedia.org/wiki/Coordinated_Universal_Time
-}
utc : Zone
utc =
  Zone 0 []


{-| Produce a `Zone` based on the current UTC offset. You can use this to figure
out what day it is where you are:

    import Task exposing (Task)
    import Time

    whatDayIsIt : Task x Int
    whatDayIsIt =
      Task.map2 Time.toDay Time.here Time.now

**Accuracy Note:** This function can only give time zones like `Etc/GMT+9` or
`Etc/GMT-6`. It cannot give you `Europe/Stockholm`, `Asia/Tokyo`, or any other
normal time zone from the [full list][tz] due to limitations in JavaScript.
For example, if you run `here` in New York City, the resulting `Zone` will
never be `America/New_York`. Instead you get `Etc/GMT-5` or `Etc/GMT-4`
depending on Daylight Saving Time. So even though browsers must have internal
access to `America/New_York` to figure out that offset, there is no public API
to get the full information. This means the `Zone` you get from this function
will act weird if (1) an application stays open across a Daylight Saving Time
boundary or (2) you try to use it on historical data.

**Future Note:** We can improve `here` when there is good browser support for
JavaScript functions that (1) expose the IANA time zone database and (2) let
you ask the time zone of the computer. The committee that reviews additions to
JavaScript is called TC39, and I encourage you to push for these capabilities! I
cannot do it myself unfortunately.

**Alternatives:** See the `customZone` docs to learn how to implement stopgaps.

[tz]: https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
-}
here : Task x Zone
here =
  Elm.Kernel.Time.here ()



-- DATES


{-| What year is it?!

    import Time exposing (toYear, utc, millisToPosix)

    toYear utc (millisToPosix 0) == 1970
    toYear nyc (millisToPosix 0) == 1969

    -- pretend `nyc` is the `Zone` for America/New_York.
-}
toYear : Zone -> Posix -> Int
toYear zone time =
  (toCivil (toAdjustedMinutes zone time)).year


{-| What month is it?!

    import Time exposing (toMonth, utc, millisToPosix)

    toMonth utc (millisToPosix 0) == Jan
    toMonth nyc (millisToPosix 0) == Dec

    -- pretend `nyc` is the `Zone` for America/New_York.
-}
toMonth : Zone -> Posix -> Month
toMonth zone time =
  case (toCivil (toAdjustedMinutes zone time)).month of
    1  -> Jan
    2  -> Feb
    3  -> Mar
    4  -> Apr
    5  -> May
    6  -> Jun
    7  -> Jul
    8  -> Aug
    9  -> Sep
    10 -> Oct
    11 -> Nov
    _  -> Dec


{-| What day is it?! (Days go from 1 to 31)

    import Time exposing (toDay, utc, millisToPosix)

    toDay utc (millisToPosix 0) == 1
    toDay nyc (millisToPosix 0) == 31

    -- pretend `nyc` is the `Zone` for America/New_York.

-}
toDay : Zone -> Posix -> Int
toDay zone time =
  (toCivil (toAdjustedMinutes zone time)).day


{-| What day of the week is it?

    import Time exposing (toWeekday, utc, millisToPosix)

    toWeekday utc (millisToPosix 0) == Thu
    toWeekday nyc (millisToPosix 0) == Wed

    -- pretend `nyc` is the `Zone` for America/New_York.
-}
toWeekday : Zone -> Posix -> Weekday
toWeekday zone time =
  case modBy 7 (flooredDiv (toAdjustedMinutes zone time) (60 * 24)) of
    0 -> Thu
    1 -> Fri
    2 -> Sat
    3 -> Sun
    4 -> Mon
    5 -> Tue
    _ -> Wed


{-| What hour is it? (From 0 to 23)

    import Time exposing (toHour, utc, millisToPosix)

    toHour utc (millisToPosix 0) == 0  -- 12am
    toHour nyc (millisToPosix 0) == 19 -- 7pm

    -- pretend `nyc` is the `Zone` for America/New_York.
-}
toHour : Zone -> Posix -> Int
toHour zone time =
  modBy 24 (flooredDiv (toAdjustedMinutes zone time) 60)


{-| What minute is it? (From 0 to 59)

    import Time exposing (toMinute, utc, millisToPosix)

    toMinute utc (millisToPosix 0) == 0

This can be different in different time zones. Some time zones are offset
by 30 or 45 minutes!
-}
toMinute : Zone -> Posix -> Int
toMinute zone time =
  modBy 60 (toAdjustedMinutes zone time)


{-| What second is it?

    import Time exposing (toSecond, utc, millisToPosix)

    toSecond utc (millisToPosix    0) == 0
    toSecond utc (millisToPosix 1234) == 1
    toSecond utc (millisToPosix 5678) == 5
-}
toSecond : Zone -> Posix -> Int
toSecond _ time =
  modBy 60 (flooredDiv (posixToMillis time) 1000)


{-|
    import Time exposing (toMillis, utc, millisToPosix)

    toMillis utc (millisToPosix    0) == 0
    toMillis utc (millisToPosix 1234) == 234
    toMillis utc (millisToPosix 5678) == 678
-}
toMillis : Zone -> Posix -> Int
toMillis _ time =
  modBy 1000 (posixToMillis time)



-- DATE HELPERS


toAdjustedMinutes : Zone -> Posix -> Int
toAdjustedMinutes (Zone defaultOffset eras) time =
  toAdjustedMinutesHelp defaultOffset (flooredDiv (posixToMillis time) 60000) eras


toAdjustedMinutesHelp : Int -> Int -> List Era -> Int
toAdjustedMinutesHelp defaultOffset posixMinutes eras =
  case eras of
    [] ->
      posixMinutes + defaultOffset

    era :: olderEras ->
      if era.start < posixMinutes then
        posixMinutes + era.offset
      else
        toAdjustedMinutesHelp defaultOffset posixMinutes olderEras


toCivil : Int -> { year : Int, month : Int, day : Int }
toCivil minutes =
  let
    rawDay    = flooredDiv minutes (60 * 24) + 719468
    era       = (if rawDay >= 0 then rawDay else rawDay - 146096) // 146097
    dayOfEra  = rawDay - era * 146097 -- [0, 146096]
    yearOfEra = (dayOfEra - dayOfEra // 1460 + dayOfEra // 36524 - dayOfEra // 146096) // 365 -- [0, 399]
    year      = yearOfEra + era * 400
    dayOfYear = dayOfEra - (365 * yearOfEra + yearOfEra // 4 - yearOfEra // 100) -- [0, 365]
    mp        = (5 * dayOfYear + 2) // 153 -- [0, 11]
    month     = mp + (if mp < 10 then 3 else -9) -- [1, 12]
  in
  { year = year + (if month <= 2 then 1 else 0)
  , month = month
  , day = dayOfYear - (153 * mp + 2) // 5 + 1 -- [1, 31]
  }


flooredDiv : Int -> Float -> Int
flooredDiv numerator denominator =
  floor (toFloat numerator / denominator)



-- WEEKDAYS AND MONTHS


{-| Represents a `Weekday` so that you can convert it to a `String` or `Int`
however you please. For example, if you need the Japanese representation, you
can say:

    toJapaneseWeekday : Weekday -> String
    toJapaneseWeekday weekday =
      case weekday of
        Mon -> "月"
        Tue -> "火"
        Wed -> "水"
        Thu -> "木"
        Fri -> "金"
        Sat -> "土"
        Sun -> "日"
-}
type Weekday = Mon | Tue | Wed | Thu | Fri | Sat | Sun


{-| Represents a `Month` so that you can convert it to a `String` or `Int`
however you please. For example, if you need the Danish representation, you
can say:

    toDanishMonth : Month -> String
    toDanishMonth month =
      case month of
        Jan -> "januar"
        Feb -> "februar"
        Mar -> "marts"
        Apr -> "april"
        May -> "maj"
        Jun -> "juni"
        Jul -> "juli"
        Aug -> "august"
        Sep -> "september"
        Oct -> "oktober"
        Nov -> "november"
        Dec -> "december"
-}
type Month = Jan | Feb | Mar | Apr | May | Jun | Jul | Aug | Sep | Oct | Nov | Dec



-- SUBSCRIPTIONS


{-| Get the current time periodically. How often though? Well, you provide an
interval in milliseconds (like `1000` for a second or `60 * 1000` for a minute
or `60 * 60 * 1000` for an hour) and that is how often you get a new time!

Check out [this example](https://elm-lang.org/examples/time) to see how to use
it in an application.

**This function is not for animation.** Use the [`elm/animation-frame`][af]
package for that sort of thing! It syncs up with repaints and will end up
being much smoother for any moving visuals.

[af]: /packages/elm/animation-frame/latest
-}
every : Float -> (Posix -> msg) -> Sub msg
every interval tagger =
  subscription (Every interval tagger)


type MySub msg =
  Every Float (Posix -> msg)


subMap : (a -> b) -> MySub a -> MySub b
subMap f (Every interval tagger) =
  Every interval (f << tagger)



-- EFFECT MANAGER


type alias State msg =
  { taggers : Taggers msg
  , processes : Processes
  }


type alias Processes =
  Dict.Dict Float Platform.ProcessId


type alias Taggers msg =
  Dict.Dict Float (List (Posix -> msg))


init : Task Never (State msg)
init =
  Task.succeed (State Dict.empty Dict.empty)


onEffects : Platform.Router msg Float -> List (MySub msg) -> State msg -> Task Never (State msg)
onEffects router subs {processes} =
  let
    newTaggers =
      List.foldl addMySub Dict.empty subs

    leftStep interval taggers (spawns, existing, kills) =
      ( interval :: spawns, existing, kills )

    bothStep interval taggers id (spawns, existing, kills) =
      ( spawns, Dict.insert interval id existing, kills )

    rightStep _ id (spawns, existing, kills) =
      ( spawns, existing, Task.andThen (\_ -> kills) (Process.kill id) )

    (spawnList, existingDict, killTask) =
      Dict.merge
        leftStep
        bothStep
        rightStep
        newTaggers
        processes
        ([], Dict.empty, Task.succeed ())
  in
    killTask
      |> Task.andThen (\_ -> spawnHelp router spawnList existingDict)
      |> Task.andThen (\newProcesses -> Task.succeed (State newTaggers newProcesses))


addMySub : MySub msg -> Taggers msg -> Taggers msg
addMySub (Every interval tagger) state =
  case Dict.get interval state of
    Nothing ->
      Dict.insert interval [tagger] state

    Just taggers ->
      Dict.insert interval (tagger :: taggers) state


spawnHelp : Platform.Router msg Float -> List Float -> Processes -> Task.Task x Processes
spawnHelp router intervals processes =
  case intervals of
    [] ->
      Task.succeed processes

    interval :: rest ->
      let
        spawnTimer =
          Process.spawn (setInterval interval (Platform.sendToSelf router interval))

        spawnRest id =
          spawnHelp router rest (Dict.insert interval id processes)
      in
        spawnTimer
          |> Task.andThen spawnRest


onSelfMsg : Platform.Router msg Float -> Float -> State msg -> Task Never (State msg)
onSelfMsg router interval state =
  case Dict.get interval state.taggers of
    Nothing ->
      Task.succeed state

    Just taggers ->
      let
        tellTaggers time =
          Task.sequence (List.map (\tagger -> Platform.sendToApp router (tagger time)) taggers)
      in
        now
          |> Task.andThen tellTaggers
          |> Task.andThen (\_ -> Task.succeed state)


setInterval : Float -> Task Never () -> Task x Never
setInterval =
  Elm.Kernel.Time.setInterval



-- FOR PACKAGE AUTHORS



{-| **Intended for package authors.**

The documentation of [`here`](#here) explains that it has certain accuracy
limitations that block on adding new APIs to JavaScript. The `customZone`
function is a stopgap that takes:

1. A default offset in minutes. So `Etc/GMT-5` is `customZone (-5 * 60) []`
and `Etc/GMT+9` is `customZone (9 * 60) []`.
2. A list of exceptions containing their `start` time in "minutes since the Unix
epoch" and their `offset` in "minutes from UTC"

Human times will be based on the nearest `start`, falling back on the default
offset if the time is older than all of the exceptions.

When paired with `getZoneName`, this allows you to load the real IANA time zone
database however you want: HTTP, cache, hardcode, etc.

**Note:** If you use this, please share your work in an Elm community forum!
I am sure others would like to hear about it, and more experience reports will
help me and the any potential TC39 proposal.
-}
customZone : Int -> List { start : Int, offset : Int } -> Zone
customZone =
  Zone


{-| **Intended for package authors.**

Use `Intl.DateTimeFormat().resolvedOptions().timeZone` to try to get names
like `Europe/Moscow` or `America/Havana`. From there you can look it up in any
IANA data you loaded yourself.
-}
getZoneName : Task x ZoneName
getZoneName =
  Elm.Kernel.Time.getZoneName ()


{-| **Intended for package authors.**

The `getZoneName` function relies on a JavaScript API that is not supported
in all browsers yet, so it can return the following:

    -- in more recent browsers
    Name "Europe/Moscow"
    Name "America/Havana"

    -- in older browsers
    Offset 180
    Offset -300

So if the real info is not available, it will tell you the current UTC offset
in minutes, just like what `here` uses to make zones like `customZone -60 []`.
-}
type ZoneName
  = Name String
  | Offset Int
