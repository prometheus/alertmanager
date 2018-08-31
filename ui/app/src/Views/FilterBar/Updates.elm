module Views.FilterBar.Updates exposing (setMatchers, update)

import Browser.Dom as Dom
import Browser.Navigation as Navigation
import Task
import Utils.Filter exposing (Filter, generateQueryString, parseFilter, stringifyFilter)
import Views.FilterBar.Types exposing (Model, Msg(..))


update : String -> Filter -> Msg -> Model -> ( Model, Cmd Msg )
update url filter msg model =
    case msg of
        AddFilterMatcher emptyMatcherText matcher ->
            immediatelyFilter url
                filter
                { model
                    | matchers =
                        if List.member matcher model.matchers then
                            model.matchers

                        else
                            model.matchers ++ [ matcher ]
                    , matcherText =
                        if emptyMatcherText then
                            ""

                        else
                            model.matcherText
                }

        DeleteFilterMatcher setMatcherText matcher ->
            immediatelyFilter url
                filter
                { model
                    | matchers = List.filter ((/=) matcher) model.matchers
                    , matcherText =
                        if setMatcherText then
                            Utils.Filter.stringifyMatcher matcher

                        else
                            model.matcherText
                }

        UpdateMatcherText value ->
            ( { model | matcherText = value }, Cmd.none )

        PressingBackspace isPressed ->
            ( { model | backspacePressed = isPressed }, Cmd.none )

        Noop ->
            ( model, Cmd.none )


immediatelyFilter : String -> Filter -> Model -> ( Model, Cmd Msg )
immediatelyFilter url filter model =
    let
        newFilter =
            { filter | text = Just (stringifyFilter model.matchers) }
    in
    ( { model | matchers = [] }
    , Cmd.batch
        [ Navigation.pushUrl model.key (url ++ generateQueryString newFilter)
        , Dom.focus "filter-bar-matcher" |> Task.attempt (always Noop)
        ]
    )


setMatchers : Filter -> Model -> Model
setMatchers filter model =
    { model
        | matchers =
            filter.text
                |> Maybe.andThen parseFilter
                |> Maybe.withDefault []
    }
