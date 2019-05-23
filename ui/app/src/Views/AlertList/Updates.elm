module Views.AlertList.Updates exposing (update)

import Alerts.Api as Api
import Browser.Navigation as Navigation
import Dict
import Set
import Types exposing (Msg(..))
import Utils.Filter exposing (Filter, generateQueryString, parseFilter)
import Utils.Types exposing (ApiData(..))
import Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..))
import Views.FilterBar.Updates as FilterBar
import Views.GroupBar.Updates as GroupBar
import Views.ReceiverBar.Updates as ReceiverBar


update : AlertListMsg -> Model -> Filter -> String -> String -> ( Model, Cmd Types.Msg )
update msg ({ groupBar, alerts, filterBar, receiverBar, alertGroups } as model) filter apiUrl basePath =
    let
        alertsUrl =
            basePath ++ "#/alerts"
    in
    case msg of
        AlertGroupsFetched listOfAlertGroups ->
            ( { model | alertGroups = listOfAlertGroups }
            , Cmd.none
            )

        AlertsFetched listOfAlerts ->
            ( { model
                | alerts = listOfAlerts
                , groupBar =
                    case listOfAlerts of
                        Success ungroupedAlerts ->
                            { groupBar
                                | list =
                                    List.concatMap (.labels >> Dict.toList) ungroupedAlerts
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
            ( { model
                | alerts =
                    if filter.customGrouping then
                        Loading

                    else
                        alerts
                , alertGroups =
                    if filter.customGrouping then
                        alertGroups

                    else
                        Loading
                , filterBar = newFilterBar
                , groupBar = newGroupBar
                , activeId = Nothing
              }
            , Cmd.batch
                [ if filter.customGrouping then
                    Api.fetchAlerts apiUrl filter |> Cmd.map (AlertsFetched >> MsgForAlertList)

                  else
                    Api.fetchAlertGroups apiUrl filter |> Cmd.map (AlertGroupsFetched >> MsgForAlertList)
                , ReceiverBar.fetchReceivers apiUrl |> Cmd.map (MsgForReceiverBar >> MsgForAlertList)
                ]
            )

        ToggleSilenced showSilenced ->
            ( model
            , Navigation.pushUrl model.key (alertsUrl ++ generateQueryString { filter | showSilenced = Just showSilenced })
            )

        ToggleInhibited showInhibited ->
            ( model
            , Navigation.pushUrl model.key (alertsUrl ++ generateQueryString { filter | showInhibited = Just showInhibited })
            )

        SetTab tab ->
            ( { model | tab = tab }, Cmd.none )

        MsgForFilterBar subMsg ->
            let
                ( newFilterBar, cmd ) =
                    FilterBar.update alertsUrl filter subMsg filterBar
            in
            ( { model | filterBar = newFilterBar, tab = FilterTab }, Cmd.map (MsgForFilterBar >> MsgForAlertList) cmd )

        MsgForGroupBar subMsg ->
            let
                ( newGroupBar, cmd ) =
                    GroupBar.update alertsUrl filter subMsg groupBar
            in
            ( { model | groupBar = newGroupBar }, Cmd.map (MsgForGroupBar >> MsgForAlertList) cmd )

        MsgForReceiverBar subMsg ->
            let
                ( newReceiverBar, cmd ) =
                    ReceiverBar.update alertsUrl filter subMsg receiverBar
            in
            ( { model | receiverBar = newReceiverBar }, Cmd.map (MsgForReceiverBar >> MsgForAlertList) cmd )

        SetActive maybeId ->
            ( { model | activeId = maybeId }, Cmd.none )
