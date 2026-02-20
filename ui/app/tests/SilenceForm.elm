module SilenceForm exposing (parseAnnotation)

import Expect
import Test exposing (..)
import Views.SilenceForm.Types


parseAnnotation : Test
parseAnnotation =
    describe "parseAnnotation"
        [ describe "valid inputs"
            [ test "should parse basic key=value" <|
                \() ->
                    Expect.equal (Just ( "key", "value" ))
                        (Views.SilenceForm.Types.parseAnnotation "key=value")
            , test "should parse value containing equals signs" <|
                \() ->
                    Expect.equal (Just ( "key", "value=extra" ))
                        (Views.SilenceForm.Types.parseAnnotation "key=value=extra")
            , test "should parse value with multiple equals signs" <|
                \() ->
                    Expect.equal (Just ( "key", "a=b=c=d" ))
                        (Views.SilenceForm.Types.parseAnnotation "key=a=b=c=d")
            , test "should trim whitespace from key and value" <|
                \() ->
                    Expect.equal (Just ( "key", "value" ))
                        (Views.SilenceForm.Types.parseAnnotation " key = value ")
            , test "should trim whitespace with equals in value" <|
                \() ->
                    Expect.equal (Just ( "key", "value=extra" ))
                        (Views.SilenceForm.Types.parseAnnotation " key = value=extra ")
            , test "should parse value with leading/trailing spaces" <|
                \() ->
                    Expect.equal (Just ( "key", "value with spaces" ))
                        (Views.SilenceForm.Types.parseAnnotation "key= value with spaces ")
            , test "should parse key with underscores and numbers" <|
                \() ->
                    Expect.equal (Just ( "annotation_key_1", "value123" ))
                        (Views.SilenceForm.Types.parseAnnotation "annotation_key_1=value123")
            , test "should allow empty value when quoted or has content after equals" <|
                \() ->
                    Expect.equal (Just ( "key", "http://example.com?foo=bar&baz=qux" ))
                        (Views.SilenceForm.Types.parseAnnotation "key=http://example.com?foo=bar&baz=qux")
            ]
        , describe "invalid inputs"
            [ test "should reject empty string" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "")
            , test "should reject string without equals sign" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "noequals")
            , test "should reject empty key" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "=value")
            , test "should reject empty value" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "key=")
            , test "should reject whitespace-only key" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "  =value")
            , test "should reject whitespace-only value" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "key=  ")
            , test "should reject both key and value as whitespace" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation " = ")
            , test "should reject only whitespace" <|
                \() ->
                    Expect.equal Nothing
                        (Views.SilenceForm.Types.parseAnnotation "   ")
            ]
        ]
