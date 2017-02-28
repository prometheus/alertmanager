module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Filter exposing (addMaybe, generateQueryString)
import Regex
import QueryString exposing (QueryString, empty, add, render)
import Navigation


update : AlertsMsg -> ApiData (List AlertGroup) -> Filter -> ( ApiData (List AlertGroup), Filter, Cmd Msg )
update msg groups filter =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( alertGroups, filter, Cmd.none )

        FetchAlertGroups ->
            ( groups, filter, Api.getAlertGroups filter )

        Noop ->
            ( groups, filter, Cmd.none )

        FilterAlerts ->
            let
                url =
                    "/#/alerts" ++ (generateQueryString filter)
            in
                ( groups, filter, generateParentMsg (NewUrl url) )


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
