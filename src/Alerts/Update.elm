module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Regex


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
                            let
                                replace =
                                    (Regex.replace Regex.All (Regex.regex "{|}|\"|\\s") (\_ -> ""))

                                matches =
                                    String.split "," (replace filter.text)

                                labels =
                                    List.filterMap
                                        (\m ->
                                            let
                                                label =
                                                    String.split "=" m
                                            in
                                                if List.length label == 2 then
                                                    Just ( Maybe.withDefault "" (List.head label), Maybe.withDefault "" (List.head <| List.reverse label) )
                                                else
                                                    Nothing
                                        )
                                        matches
                            in
                                -- Instead of changing filter, change the query string and that then is parsed into the filter structure
                                { filter | labels = labels }

                        _ ->
                            filter
            in
                ( groups, f, Cmd.none )


urlUpdate : Route -> AlertsMsg
urlUpdate _ =
    FetchAlertGroups


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


filterAlertsByLabel : List ( String, String ) -> Block -> Maybe Block
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


filterAlertGroupLabels : List ( String, String ) -> AlertGroup -> Maybe AlertGroup
filterAlertGroupLabels labels alertGroup =
    let
        blocks =
            List.filterMap (filterAlertsByLabel labels) alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing


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
    filterAlertsFromBlock (\a -> not a.silenced) block


filterByLabels : List ( String, String ) -> List AlertGroup -> List AlertGroup
filterByLabels labels groups =
    filterBy (filterAlertGroupLabels labels) groups
