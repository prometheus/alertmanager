module Views.FilterBar.Updates exposing (setMatchers, update)

import Browser.Dom as Dom
import Task
import Utils.Filter exposing (Filter, parseFilter)
import Views.FilterBar.Types exposing (Model, Msg(..))


{-| Returns a triple where the Bool component notifies whether the matchers have changed.
-}
update : Msg -> Model -> ( Model, Bool, Cmd Msg )
update msg model =
    case msg of
        AddFilterMatcher emptyMatcherText matcher ->
            ( { model
                | matchers =
                    if List.member matcher model.matchers then
                        model.matchers

                    else
                        model.matchers ++ [ matcher ]
                , keepMatchers =
                    case model.editing of
                        Just edited ->
                            List.map
                                (\existing ->
                                    if existing == edited then
                                        matcher

                                    else
                                        existing
                                )
                                model.keepMatchers

                        Nothing ->
                            model.keepMatchers
                , editing = Nothing
                , matcherText =
                    if emptyMatcherText then
                        ""

                    else
                        model.matcherText
              }
            , True
            , Dom.focus "filter-bar-matcher"
                |> Task.attempt (always Noop)
            )

        DeleteFilterMatcher setMatcherText matcher ->
            ( { model
                | matchers = List.filter ((/=) matcher) model.matchers
                , keepMatchers =
                    if setMatcherText then
                        -- Editing a matcher keeps its pin so it can be transferred to the
                        -- edited matcher. Drop a previously edited matcher that was never
                        -- re-added, though.
                        case model.editing of
                            Just previous ->
                                List.filter ((/=) previous) model.keepMatchers

                            Nothing ->
                                model.keepMatchers

                    else
                        List.filter ((/=) matcher) model.keepMatchers
                , editing =
                    if setMatcherText then
                        Just matcher

                    else
                        model.editing
                , matcherText =
                    if setMatcherText then
                        Utils.Filter.stringifyMatcher matcher

                    else
                        model.matcherText
              }
            , True
            , Dom.focus "filter-bar-matcher"
                |> Task.attempt (always Noop)
            )

        KeepFilterMatcher matcher ->
            ( { model
                | keepMatchers =
                    if List.member matcher model.keepMatchers then
                        List.filter ((/=) matcher) model.keepMatchers

                    else
                        model.keepMatchers ++ [ matcher ]
              }
            , True
            , Cmd.none
            )

        UpdateMatcherText value ->
            ( { model | matcherText = value }, False, Cmd.none )

        PressingBackspace isPressed ->
            ( { model | backspacePressed = isPressed }, False, Cmd.none )

        Noop ->
            ( model, False, Cmd.none )


setMatchers : Filter -> Model -> Model
setMatchers filter model =
    { model
        | matchers =
            filter.text
                |> Maybe.andThen parseFilter
                |> Maybe.withDefault []
        , keepMatchers = []
        , editing = Nothing
    }
