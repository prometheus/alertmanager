module Filter exposing (parseFilter, parseMatcher, stringifyFilter, toUrl)

import Expect
import Fuzz exposing (string, tuple)
import Helpers exposing (isNotEmptyTrimmedAlphabetWord)
import Test exposing (..)
import Utils.Filter exposing (MatchOperator(..), Matcher)


parseMatcher : Test
parseMatcher =
    describe "parseMatcher"
        [ test "should parse empty matcher string" <|
            \() ->
                Expect.equal Nothing (Utils.Filter.parseMatcher "")
        , test "should parse empty matcher value" <|
            \() ->
                Expect.equal (Just (Matcher "alertname" Eq "")) (Utils.Filter.parseMatcher "alertname=\"\"")
        , test "should unescape quoted matcher value" <|
            \() ->
                Expect.equal
                    (Just (Matcher "alertname" Eq "foo\"bar"))
                    (Utils.Filter.parseMatcher "alertname=\"foo\\\"bar\"")
        , test "should unescape backslash matcher value" <|
            \() ->
                Expect.equal
                    (Just (Matcher "alertname" Eq "foo\\bar"))
                    (Utils.Filter.parseMatcher "alertname=\"foo\\\\bar\"")
        , test "should parse dotted label name" <|
            \() ->
                Expect.equal
                    (Just (Matcher "host.name" Eq "server-01"))
                    (Utils.Filter.parseMatcher "host.name=\"server-01\"")
        , test "should parse label name with multiple dots" <|
            \() ->
                Expect.equal
                    (Just (Matcher "service.namespace.name" Eq "myapp"))
                    (Utils.Filter.parseMatcher "service.namespace.name=\"myapp\"")
        , test "should parse dotted label name with regex match" <|
            \() ->
                Expect.equal
                    (Just (Matcher "host.name" RegexMatch "server-.*"))
                    (Utils.Filter.parseMatcher "host.name=~\"server-.*\"")
        , test "should parse dotted label name with not equal" <|
            \() ->
                Expect.equal
                    (Just (Matcher "host.type" NotEq "test"))
                    (Utils.Filter.parseMatcher "host.type!=\"test\"")
        , test "should parse label name with hyphens" <|
            \() ->
                Expect.equal
                    (Just (Matcher "cluster-id" Eq "prod-us-east"))
                    (Utils.Filter.parseMatcher "cluster-id=\"prod-us-east\"")
        , test "should parse dotted label name with not regex match" <|
            \() ->
                Expect.equal
                    (Just (Matcher "host.name" NotRegexMatch "server-.*"))
                    (Utils.Filter.parseMatcher "host.name!~\"server-.*\"")
        , test "should parse UTF-8 label name" <|
            \() ->
                Expect.equal
                    (Just (Matcher "Προμηθεύς" Eq "alerts"))
                    (Utils.Filter.parseMatcher "Προμηθεύς=\"alerts\"")
        , test "should reject NBSP in label name" <|
            \() ->
                let
                    nbsp =
                        String.fromChar (Char.fromCode 0xA0)
                in
                Expect.equal
                    Nothing
                    (Utils.Filter.parseMatcher ("host" ++ nbsp ++ "name=\"server-01\""))
        , fuzz (tuple ( string, string )) "should parse random matcher string" <|
            \( key, value ) ->
                if List.map isNotEmptyTrimmedAlphabetWord [ key, value ] /= [ True, True ] then
                    Expect.equal
                        Nothing
                        (Utils.Filter.parseMatcher <| String.concat [ key, "=", value ])

                else
                    Expect.equal
                        (Just (Matcher key Eq value))
                        (Utils.Filter.parseMatcher <| String.concat [ key, "=", "\"", value, "\"" ])
        ]


toUrl : Test
toUrl =
    describe "toUrl"
        [ test "should not render keys with Nothing value except the silenced, inhibited, muted and active parameters, which default to false, false, false, true, respectively." <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=false&active=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Nothing, showSilenced = Nothing, showInhibited = Nothing, showMuted = Nothing, showActive = Nothing })
        , test "should not render filter key with empty value" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=false&active=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Just "", showSilenced = Nothing, showInhibited = Nothing, showMuted = Nothing, showActive = Nothing })
        , test "should render filter key with values" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=false&active=true&filter=%7Bfoo%3D%22bar%22%2C%20baz%3D~%22quux.*%22%7D"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Just "{foo=\"bar\", baz=~\"quux.*\"}", showSilenced = Nothing, showInhibited = Nothing, showMuted = Nothing, showActive = Nothing })
        , test "should render silenced key with bool" <|
            \() ->
                Expect.equal "/alerts?silenced=true&inhibited=false&muted=false&active=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Nothing, showSilenced = Just True, showInhibited = Nothing, showMuted = Nothing, showActive = Nothing })
        , test "should render inhibited key with bool" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=true&muted=false&active=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Nothing, showSilenced = Nothing, showInhibited = Just True, showMuted = Nothing, showActive = Nothing })
        , test "should render muted key with bool" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=true&active=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Nothing, showSilenced = Nothing, showInhibited = Nothing, showMuted = Just True, showActive = Nothing })
        , test "should render active key with bool" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=false&active=false"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = False, text = Nothing, showSilenced = Nothing, showInhibited = Nothing, showMuted = Nothing, showActive = Just False })
        , test "should add customGrouping key" <|
            \() ->
                Expect.equal "/alerts?silenced=false&inhibited=false&muted=false&active=true&customGrouping=true"
                    (Utils.Filter.toUrl "/alerts" { receiver = Nothing, group = Nothing, customGrouping = True, text = Nothing, showSilenced = Nothing, showInhibited = Nothing, showMuted = Nothing, showActive = Nothing })
        ]


parseFilter : Test
parseFilter =
    describe "parseFilter"
        [ test "should parse filter with dotted label names" <|
            \() ->
                Expect.equal
                    (Just
                        [ Matcher "host.name" Eq "server-01"
                        , Matcher "service.namespace" Eq "myapp"
                        ]
                    )
                    (Utils.Filter.parseFilter "{host.name=\"server-01\", service.namespace=\"myapp\"}")
        , test "should parse filter with mixed label names" <|
            \() ->
                Expect.equal
                    (Just
                        [ Matcher "alertname" Eq "MyAlert"
                        , Matcher "host.name" Eq "server-01"
                        ]
                    )
                    (Utils.Filter.parseFilter "{alertname=\"MyAlert\", host.name=\"server-01\"}")
        ]


stringifyFilter : Test
stringifyFilter =
    describe "stringifyFilter"
        [ test "empty" <|
            \() ->
                Expect.equal ""
                    (Utils.Filter.stringifyFilter [])
        , test "non-empty" <|
            \() ->
                Expect.equal "{foo=\"bar\", baz=~\"quux.*\"}"
                    (Utils.Filter.stringifyFilter
                        [ { key = "foo", op = Eq, value = "bar" }
                        , { key = "baz", op = RegexMatch, value = "quux.*" }
                        ]
                    )
        , test "escapes matcher values" <|
            \() ->
                Expect.equal "{foo=\"bar\\\"baz\\\\qux\"}"
                    (Utils.Filter.stringifyFilter
                        [ { key = "foo", op = Eq, value = "bar\"baz\\qux" } ]
                    )
        ]
