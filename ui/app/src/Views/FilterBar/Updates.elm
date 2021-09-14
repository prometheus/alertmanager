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
    }
