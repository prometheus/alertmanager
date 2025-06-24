port module Views.AlertList.Updates exposing (update)

import Alerts.Api as Api
import Browser.Navigation as Navigation
import Data.AlertGroup exposing (AlertGroup)
import Dict
import Set
import Task
import Types exposing (Msg(..))
import Utils.Filter exposing (Filter)
import Utils.List
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

        filteredUrl =
            Utils.Filter.toUrl alertsUrl
    in
    case msg of
        AlertGroupsFetched listOfAlertGroups ->
            ( { model | alertGroups = listOfAlertGroups }
            , Cmd.none
            )

        AlertsFetched listOfAlerts ->
            let
                ( groups_, groupBar_ ) =
                    case listOfAlerts of
                        Success ungroupedAlerts ->
                            let
                                groups =
                                    ungroupedAlerts
                                        |> Utils.List.groupBy
                                            (.labels >> Dict.toList >> List.filter (\( key, _ ) -> List.member key groupBar.fields))
                                        |> Dict.toList
                                        |> List.map
                                            (\( labels, alerts_ ) ->
                                                AlertGroup (Dict.fromList labels) { name = "unknown" } alerts_
                                            )

                                newGroupBar =
                                    { groupBar
                                        | list =
                                            List.concatMap (.labels >> Dict.toList) ungroupedAlerts
                                                |> List.map Tuple.first
                                                |> Set.fromList
                                    }
                            in
                            ( Success groups, newGroupBar )

                        Initial ->
                            ( Initial, groupBar )

                        Loading ->
                            ( Loading, groupBar )

                        Failure e ->
                            ( Failure e, groupBar )
            in
            ( { model
                | alerts = listOfAlerts
                , alertGroups = groups_
                , groupBar = groupBar_
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
                , activeGroups = Set.empty
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
            , Navigation.pushUrl model.key (filteredUrl { filter | showSilenced = Just showSilenced })
            )

        ToggleInhibited showInhibited ->
            ( model
            , Navigation.pushUrl model.key (filteredUrl { filter | showInhibited = Just showInhibited })
            )

        ToggleMuted showMuted ->
            ( model
            , Navigation.pushUrl model.key (filteredUrl { filter | showMuted = Just showMuted })
            )

        SetTab tab ->
            ( { model | tab = tab }, Cmd.none )

        MsgForFilterBar subMsg ->
            let
                ( newFilterBar, shouldFilter, cmd ) =
                    FilterBar.update subMsg filterBar

                filterBarCmd =
                    Cmd.map (MsgForFilterBar >> MsgForAlertList) cmd

                newUrl =
                    filteredUrl (Utils.Filter.withMatchers newFilterBar.matchers filter)

                alertsCmd =
                    if shouldFilter then
                        Cmd.batch
                            [ Navigation.pushUrl model.key newUrl
                            , filterBarCmd
                            ]

                    else
                        filterBarCmd
            in
            ( { model | filterBar = newFilterBar, tab = FilterTab }, alertsCmd )

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

        ActiveGroups activeGroup ->
            let
                activeGroups_ =
                    if Set.member activeGroup model.activeGroups then
                        Set.remove activeGroup model.activeGroups

                    else
                        Set.insert activeGroup model.activeGroups
            in
            ( { model | activeGroups = activeGroups_, expandAll = False }, persistGroupExpandAll False )

        ToggleExpandAll expanded ->
            let
                allGroupLabels =
                    case ( alertGroups, expanded ) of
                        ( Success groups, True ) ->
                            List.range 0 (List.length groups)
                                |> Set.fromList

                        _ ->
                            Set.empty
            in
            ( { model
                | expandAll = expanded
                , activeGroups = allGroupLabels
              }
            , Cmd.batch
                [ persistGroupExpandAll expanded
                , Task.succeed expanded |> Task.perform SetGroupExpandAll
                ]
            )


port persistGroupExpandAll : Bool -> Cmd msg
