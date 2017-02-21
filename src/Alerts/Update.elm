module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Parsing
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

                query =
                    empty
                        |> addMaybe "receiver" filter.receiver identity
                        |> addMaybe "silenced" filter.showSilenced (toString >> String.toLower)
                        |> addMaybe "filter" filter.text identity
                        |> render

                url =
                    "/#/alerts" ++ query
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


addMaybe : String -> Maybe a -> (a -> String) -> QueryString -> QueryString
addMaybe key maybeValue stringFn qs =
    case maybeValue of
        Just value ->
            add key (stringFn value) qs

        Nothing ->
            qs


urlUpdate : Route -> ( AlertsMsg, Filter )
urlUpdate route =
    ( FetchAlertGroups, updateFilter route )


filterBy : (a -> Maybe a) -> List a -> List a
filterBy fn groups =
    List.filterMap fn groups


filterByReceiver : Maybe String -> List AlertGroup -> List AlertGroup
filterByReceiver maybeReceiver groups =
    case maybeReceiver of
        Just receiver ->
            filterBy (filterAlertGroup receiver) groups

        Nothing ->
            groups


filterAlertGroup : String -> AlertGroup -> Maybe AlertGroup
filterAlertGroup receiver alertGroup =
    let
        blocks =
            List.filter (\b -> receiver == b.routeOpts.receiver) alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing


filterBySilenced : Maybe Bool -> List AlertGroup -> List AlertGroup
filterBySilenced maybeShowSilenced groups =
    case maybeShowSilenced of
        Just showSilenced ->
            groups

        Nothing ->
            filterBy filterAlertGroupSilenced groups


filterAlertsFromBlock : (Alert -> Bool) -> Block -> Maybe Block
filterAlertsFromBlock fn block =
    let
        alerts =
            List.filter fn block.alerts
    in
        if not <| List.isEmpty alerts then
            Just { block | alerts = alerts }
        else
            Nothing


filterAlertsByLabel : Utils.Types.Labels -> Block -> Maybe Block
filterAlertsByLabel labels block =
    filterAlertsFromBlock
        (\a ->
            -- Check that all labels are present within the alert's label set.
            List.all
                (\l ->
                    List.member l a.labels
                )
                labels
        )
        block


filterAlertGroupLabels : Utils.Types.Matchers -> AlertGroup -> Maybe AlertGroup
filterAlertGroupLabels matchers alertGroup =
    let
        blocks =
            List.filterMap (filterAlertsByLabel <| matchersToLabels matchers) alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing


matchersToLabels : Utils.Types.Matchers -> Utils.Types.Labels
matchersToLabels matchers =
    List.map (\m -> ( m.name, m.value )) matchers


filterAlertGroupSilenced : AlertGroup -> Maybe AlertGroup
filterAlertGroupSilenced alertGroup =
    let
        blocks =
            List.filterMap filterSilencedAlerts alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing


filterSilencedAlerts : Block -> Maybe Block
filterSilencedAlerts block =
    filterAlertsFromBlock (.silenced >> not) block


filterByLabels : Maybe Utils.Types.Matchers -> List AlertGroup -> List AlertGroup
filterByLabels labels groups =
    case labels of
        Just ls ->
            filterBy (filterAlertGroupLabels ls) groups

        Nothing ->
            groups
