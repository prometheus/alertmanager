module SilenceForm exposing (parseAnnotation, toSilence, validateForm)

import Expect
import Test exposing (..)
import Time
import Utils.Date
import Utils.DateTimePicker.Types exposing (initDateTimePicker)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Utils.Filter
import Utils.FormValidation exposing (ValidationState(..), initialField)
import Views.FilterBar.Types as FilterBar
import Views.SilenceForm.Types


silenceForm : String -> String -> Views.SilenceForm.Types.SilenceForm
silenceForm createdBy comment =
    let
        startsAt =
            Utils.Date.timeToString (Time.millisToPosix 1000000000000)

        endsAt =
            Utils.Date.timeToString (Time.millisToPosix 1000003600000)
    in
    { id = Nothing
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField startsAt
    , endsAt = initialField endsAt
    , duration = initialField "1h"
    , dateTimePicker = initDateTimePicker Monday
    , viewDateTimePicker = False
    , annotations = []
    , annotationText = ""
    }


matcherFilterBar : List Utils.Filter.Matcher -> FilterBar.Model
matcherFilterBar matchers =
    FilterBar.initFilterBar matchers


toSilence : Test
toSilence =
    describe "toSilence"
        [ test "accepts an empty creator and comment" <|
            \() ->
                Expect.notEqual Nothing
                    (Views.SilenceForm.Types.toSilence
                        (matcherFilterBar [ { key = "alertname", op = Utils.Filter.Eq, value = "ExampleAlert" } ])
                        (silenceForm "" "")
                    )
        , test "accepts a creator and comment" <|
            \() ->
                Expect.notEqual Nothing
                    (Views.SilenceForm.Types.toSilence
                        (matcherFilterBar [ { key = "alertname", op = Utils.Filter.Eq, value = "ExampleAlert" } ])
                        (silenceForm "alice" "maintenance window")
                    )
        , test "still requires at least one matcher" <|
            \() ->
                Expect.equal Nothing
                    (Views.SilenceForm.Types.toSilence
                        (matcherFilterBar [])
                        (silenceForm "" "")
                    )
        ]


validateForm : Test
validateForm =
    describe "validateForm"
        [ test "does not flag an empty creator and comment" <|
            \() ->
                let
                    form =
                        Views.SilenceForm.Types.validateForm (silenceForm "" "")

                    isInvalid state =
                        case state of
                            Invalid _ ->
                                True

                            _ ->
                                False
                in
                Expect.equal ( False, False )
                    ( isInvalid form.createdBy.validationState
                    , isInvalid form.comment.validationState
                    )
        , test "still flags invalid start and end times" <|
            \() ->
                let
                    baseForm =
                        silenceForm "" ""

                    form =
                        Views.SilenceForm.Types.validateForm
                            { baseForm | startsAt = initialField "not-a-time" }

                    isInvalid state =
                        case state of
                            Invalid _ ->
                                True

                            _ ->
                                False
                in
                Expect.equal True (isInvalid form.startsAt.validationState)
        ]


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
            , test "should parse value containing URL with query parameters" <|
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
