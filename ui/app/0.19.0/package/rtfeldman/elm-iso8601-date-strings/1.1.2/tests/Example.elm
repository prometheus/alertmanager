module Example exposing (knownValues, reflexive)

import Expect exposing (Expectation)
import Fuzz exposing (Fuzzer, float, int, list, string)
import Iso8601
import Test exposing (..)
import Time


knownValues : Test
knownValues =
    describe "Epoch"
        [ test "fromTime 0 is January 1, 1970 at midnight" <|
            \_ ->
                Iso8601.fromTime (Time.millisToPosix 0)
                    |> Expect.equal "1970-01-01T00:00:00.000Z"
        , test "toTime \"1970-01-01T00:00:00.000Z\" gives me 0" <|
            \_ ->
                Iso8601.toTime "1970-01-01T00:00:00.000Z"
                    |> Expect.equal (Ok (Time.millisToPosix 0))
        , test "toTime \"1970-01-01T00:00:00Z\" gives me 0" <|
            \_ ->
                Iso8601.toTime "1970-01-01T00:00:00Z"
                    |> Expect.equal (Ok (Time.millisToPosix 0))
        , test "toTime \"2012-04-01T00:00:00-05:00\" gives me 1333256400000" <|
            \_ ->
                Iso8601.toTime "2012-04-01T00:00:00-05:00"
                    |> Expect.equal (Ok (Time.millisToPosix 1333256400000))
        , test "toTime \"2012-11-12T00:00:00+01:00\" gives me 1352674800000" <|
            \_ ->
                Iso8601.toTime "2012-11-12T00:00:00+01:00"
                    |> Expect.equal (Ok (Time.millisToPosix 1352674800000))
        , test "-5:00 is an invalid UTC offset (should be -05:00)" <|
            \_ ->
                Iso8601.toTime "2012-04-01T00:00:00-5:00"
                    |> Expect.err
        , test "+1:00 is an invalid UTC offset (should be +01:00)" <|
            \_ ->
                Iso8601.toTime "2012-04-01T00:00:00+1:00"
                    |> Expect.err
        , test "Invalid timestamps don't parse" <|
            \_ ->
                Iso8601.toTime "2012-04-01T00:basketball"
                    |> Expect.err
        , test "toTime \"1970-01-01\" gives me 0" <|
            \_ ->
                Iso8601.toTime "1970-01-01"
                    |> Expect.equal (Ok (Time.millisToPosix 0))
        , test "toTime supports microseconds precision" <|
            \_ ->
                Iso8601.toTime "2018-08-31T23:25:16.019345+02:00"
                    |> Expect.equal (Ok (Time.millisToPosix 1535750716019))
        , test "toTime supports nanoseconds precision" <|
            \_ ->
                Iso8601.toTime "2018-08-31T23:25:16.019345123+02:00"
                    |> Expect.equal (Ok (Time.millisToPosix 1535750716019))
        , test "toTime doesn't support fractions more than 9 digits" <|
            \_ ->
                Iso8601.toTime "2018-08-31T23:25:16.0123456789+02:00"
                    |> Expect.err
        ]


reflexive : Test
reflexive =
    fuzz int "(fromTime >> toTime) is a no-op" <|
        \num ->
            let
                time =
                    Time.millisToPosix num
            in
            Iso8601.fromTime time
                |> Iso8601.toTime
                |> Expect.equal (Ok time)
