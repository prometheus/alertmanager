module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Model)
import Views.FilterBar.Updates as FilterBar
import Utils.Filter exposing (Filter, parseFilter)
import Utils.Types exposing (ApiData, ApiResponse(Initial, Loading, Success, Failure))
import Types exposing (Msg(MsgForAlertList, Noop))
import Set
import Views.GroupBar.Updates as GroupBar


update : AlertListMsg -> Model -> Filter -> ( Model, Cmd Types.Msg )
update msg ({ groupBar, filterBar } as model) filter =
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

                newModel =
                    { model | filterBar = newFilterBar, groupBar = newGroupBar }
            in
                -- only fetch when filter changed
                if newFilterBar /= model.filterBar || model.alerts == Initial then
                    ( { newModel | alerts = Loading }
                    , Api.fetchAlerts filter |> Cmd.map (AlertsFetched >> MsgForAlertList)
                    )
                else
                    ( newModel, Cmd.none )

        MsgForFilterBar msg ->
            let
                ( newFilterBar, cmd ) =
                    FilterBar.update "/#/alerts" filter msg filterBar
            in
                ( { model | filterBar = newFilterBar }, Cmd.map (MsgForFilterBar >> MsgForAlertList) cmd )

        MsgForGroupBar msg ->
            let
                ( newGroupBar, cmd ) =
                    GroupBar.update "/#/alerts" filter msg groupBar
            in
                ( { model | groupBar = newGroupBar }, Cmd.map (MsgForGroupBar >> MsgForAlertList) cmd )
