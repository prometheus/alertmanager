module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Filter exposing (generateQueryString)


update : AlertsMsg -> ApiData (List AlertGroup) -> Filter -> ( ApiData (List AlertGroup), Cmd Msg )
update msg groups filter =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( alertGroups, Cmd.none )

        FetchAlertGroups ->
            ( groups, Api.getAlertGroups filter )

        Noop ->
            ( groups, Cmd.none )

        FilterAlerts ->
            let
                url =
                    "/#/alerts" ++ generateQueryString filter
            in
                ( groups, generateParentMsg (NewUrl url) )


generateParentMsg : OutMsg -> Cmd Msg
generateParentMsg outMsg =
    Task.perform ForParent (Task.succeed outMsg)


updateFilter : Route -> Filter
updateFilter route =
    case route of
        Receiver maybeReceiver maybeShowSilenced maybeFilter ->
            { receiver = maybeReceiver
            , showSilenced = maybeShowSilenced
            , text = maybeFilter
            }


urlUpdate : Route -> ( AlertsMsg, Filter )
urlUpdate route =
    ( FetchAlertGroups, updateFilter route )
