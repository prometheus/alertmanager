module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task


update : AlertsMsg -> List AlertGroup -> ( List AlertGroup, Cmd Msg )
update msg groups =
    case msg of
        AlertGroupsFetch (Ok alertGroups) ->
            ( alertGroups, Cmd.none )

        AlertGroupsFetch (Err err) ->
            ( groups, Cmd.none )

        FetchAlertGroups ->
            ( groups, Api.getAlertGroups )

        Noop ->
            ( groups, Cmd.none )


urlUpdate : Route -> AlertsMsg
urlUpdate _ =
    FetchAlertGroups
