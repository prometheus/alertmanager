module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Model)
import Navigation
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Filter exposing (generateQueryString, stringifyFilter, parseFilter)
import Types exposing (Msg(MsgForAlertList, Noop))
import Dom
import Task


immediatelyFilter : Filter -> Model -> ( Model, Cmd Types.Msg )
immediatelyFilter filter model =
    let
        newFilter =
            { filter | text = Just (stringifyFilter model.matchers) }
    in
        ( model
        , Cmd.batch
            [ Navigation.newUrl ("/#/alerts" ++ generateQueryString newFilter)
            , Dom.focus "custom-matcher" |> Task.attempt (always Noop)
            ]
        )


update : AlertListMsg -> Model -> Filter -> ( Model, Cmd Types.Msg )
update msg model filter =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( { model | alertGroups = alertGroups }, Cmd.none )

        FetchAlertGroups ->
            ( { model
                | matchers =
                    filter.text
                        |> Maybe.andThen parseFilter
                        |> Maybe.withDefault []
                , alertGroups = Loading
              }
            , Api.alertGroups filter |> Cmd.map (AlertGroupsFetch >> MsgForAlertList)
            )

        AddFilterMatcher emptyMatcherText matcher ->
            immediatelyFilter filter
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
            immediatelyFilter filter
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
