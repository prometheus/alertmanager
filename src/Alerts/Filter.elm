module Alerts.Filter exposing (receiver, silenced, labels)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Utils.Types exposing (Matchers)


by : (a -> Maybe a) -> List a -> List a
by fn groups =
    List.filterMap fn groups


receiver : Maybe String -> List AlertGroup -> List AlertGroup
receiver maybeReceiver groups =
    case maybeReceiver of
        Just receiver ->
            by (filterAlertGroup receiver) groups

        Nothing ->
            groups


labels : Maybe Utils.Types.Matchers -> List AlertGroup -> List AlertGroup
labels labels groups =
    case labels of
        Just ls ->
            by (filterAlertGroupLabels ls) groups

        Nothing ->
            groups


silenced : Maybe Bool -> List AlertGroup -> List AlertGroup
silenced maybeShowSilenced groups =
    let
        showSilenced =
            Maybe.withDefault False maybeShowSilenced
    in
        if showSilenced then
            groups
        else
            by filterAlertGroupSilenced groups


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
