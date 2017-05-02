module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Model)
import Navigation
import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Utils.Filter exposing (Filter, generateQueryString, stringifyFilter, parseFilter)
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
        AlertsFetched listOfAlerts ->
            ( { model | alerts = listOfAlerts }, Cmd.none )

        FetchAlerts ->
            ( { model
                | matchers =
                    filter.text
                        |> Maybe.andThen parseFilter
                        |> Maybe.withDefault []
                , alerts = Loading
              }
            , Api.fetchAlerts filter |> Cmd.map (AlertsFetched >> MsgForAlertList)
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
