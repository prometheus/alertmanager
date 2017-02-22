module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Parsing
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
            ( groups, filter, Api.getAlertGroups )

        Noop ->
            ( groups, filter, Cmd.none )

        FilterAlerts ->
            let
                f =
                    case groups of
                        Success groups ->
                            { filter | matchers = Utils.Parsing.parseLabels filter.text }

                        _ ->
                            filter

                url =
                    "/#/alerts" ++ (generateQueryString filter)
            in
                ( groups, f, generateParentMsg (NewUrl url) )


generateParentMsg : OutMsg -> Cmd Msg
generateParentMsg outMsg =
    Task.perform ForParent (Task.succeed outMsg)


updateFilter : Route -> Filter
updateFilter route =
    case route of
        Receiver maybeReceiver maybeShowSilenced maybeQuery ->
            { receiver = maybeReceiver
            , showSilenced = maybeShowSilenced
            , text = maybeQuery
            , matchers = Utils.Parsing.parseLabels maybeQuery
            }


urlUpdate : Route -> ( AlertsMsg, Filter )
urlUpdate route =
    ( FetchAlertGroups, updateFilter route )
