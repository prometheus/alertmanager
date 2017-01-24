module Alerts.Update exposing (..)

import Alerts.Types exposing (..)
import Alerts.Api as Api


update : AlertsMsg -> List AlertGroup -> ( List AlertGroup, Maybe Alert, Maybe Bool, Cmd AlertsMsg )
update msg groups =
    case msg of
        AlertGroupsFetch (Ok alertGroups) ->
            ( alertGroups, Nothing, Just False, Cmd.none )

        AlertGroupsFetch (Err err) ->
            ( groups, Nothing, Just False, Cmd.none )

        FetchAlertGroups ->
            ( groups, Nothing, Just True, Api.getAlertGroups )

        SendAlert alert ->
            ( groups, Just alert, Nothing, Cmd.none )

        Noop ->
            ( groups, Nothing, Nothing, Cmd.none )


urlUpdate : Route -> AlertsMsg
urlUpdate _ =
    FetchAlertGroups
