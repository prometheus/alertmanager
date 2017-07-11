module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(FilterTab, GroupTab))
import Views.FilterBar.Updates as FilterBar
import Utils.Filter exposing (Filter, parseFilter)
import Utils.Types exposing (ApiData(Initial, Loading, Success, Failure))
import Types exposing (Msg(MsgForAlertList, Noop))
import Set
import Navigation
import Utils.Filter exposing (generateQueryString)
import Views.GroupBar.Updates as GroupBar
import Views.ReceiverBar.Updates as ReceiverBar


update : AlertListMsg -> Model -> Filter -> String -> String -> ( Model, Cmd Types.Msg )
update msg ({ groupBar, filterBar, receiverBar } as model) filter apiUrl basePath =
    let
        alertsUrl =
            basePath ++ "#/alerts"
    in
        case msg of
            AlertsFetched listOfAlerts ->
                ( { model
                    | alerts = listOfAlerts
                    , groupBar =
                        case listOfAlerts of
                            Success alerts ->
                                { groupBar
                                    | list =
                                        List.concatMap .labels alerts
                                            |> List.map Tuple.first
                                            |> Set.fromList
                                }

                            _ ->
                                groupBar
                  }
                , Cmd.none
                )

            FetchAlerts ->
                let
                    newGroupBar =
                        GroupBar.setFields filter groupBar

                    newFilterBar =
                        FilterBar.setMatchers filter filterBar
                in
                    ( { model | alerts = Loading, filterBar = newFilterBar, groupBar = newGroupBar, activeId = Nothing }
                    , Cmd.batch
                        [ Api.fetchAlerts apiUrl filter |> Cmd.map (AlertsFetched >> MsgForAlertList)
                        , ReceiverBar.fetchReceivers apiUrl |> Cmd.map (MsgForReceiverBar >> MsgForAlertList)
                        ]
                    )

            ToggleSilenced showSilenced ->
                ( model
                , Navigation.newUrl (alertsUrl ++ generateQueryString { filter | showSilenced = Just showSilenced })
                )

            SetTab tab ->
                ( { model | tab = tab }, Cmd.none )

            MsgForFilterBar msg ->
                let
                    ( newFilterBar, cmd ) =
                        FilterBar.update alertsUrl filter msg filterBar
                in
                    ( { model | filterBar = newFilterBar, tab = FilterTab }, Cmd.map (MsgForFilterBar >> MsgForAlertList) cmd )

            MsgForGroupBar msg ->
                let
                    ( newGroupBar, cmd ) =
                        GroupBar.update alertsUrl filter msg groupBar
                in
                    ( { model | groupBar = newGroupBar }, Cmd.map (MsgForGroupBar >> MsgForAlertList) cmd )

            MsgForReceiverBar msg ->
                let
                    ( newReceiverBar, cmd ) =
                        ReceiverBar.update alertsUrl filter msg receiverBar
                in
                    ( { model | receiverBar = newReceiverBar }, Cmd.map (MsgForReceiverBar >> MsgForAlertList) cmd )

            SetActive maybeId ->
                ( { model | activeId = maybeId }, Cmd.none )
