module Views.FilterBar.Updates exposing (setMatchers, update)

import Browser.Dom as Dom
import Task
import Utils.Filter exposing (Filter, parseCreatedByList, parseFilter)
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

        AddFilterCreatedBy emptyCreatedByText createdBy ->
            ( { model
                | createdByList =
                    if List.member createdBy model.createdByList then
                        model.createdByList

                    else
                        model.createdByList ++ [ createdBy ]
                , createdByText =
                    if emptyCreatedByText then
                        ""

                    else
                        model.createdByText
              }
            , True
            , Dom.focus "filter-bar-created-by"
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

        DeleteFilterCreatedBy setCreatedByText createdBy ->
            ( { model
                | createdByList = List.filter ((/=) createdBy) model.createdByList
                , createdByText =
                    if setCreatedByText then
                        createdBy

                    else
                        model.createdByText
              }
            , True
            , Dom.focus "filter-bar-created-by"
                |> Task.attempt (always Noop)
            )

        UpdateMatcherText value ->
            ( { model | matcherText = value }, False, Cmd.none )

        PressingBackspace isPressed ->
            ( { model | backspacePressed = isPressed }, False, Cmd.none )

        UpdateCreatedByText value ->
            ( { model | createdByText = value }, False, Cmd.none )

        ShowCreatedByBar ->
            ( { model
                | showCreatedByBar =
                    if model.showCreatedByBar then
                        False

                    else
                        True
              }
            , False
            , Cmd.none
            )

        Noop ->
            ( model, False, Cmd.none )


setMatchers : Filter -> Model -> Model
setMatchers filter model =
    { model
        | matchers =
            filter.text
                |> Maybe.andThen parseFilter
                |> Maybe.withDefault []
        , createdByList =
            filter.createdByList
                |> Maybe.andThen parseCreatedByList
                |> Maybe.withDefault []
    }
