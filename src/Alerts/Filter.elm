module Alerts.Filter exposing (receiver, silenced, matchers)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Utils.Types exposing (Matchers)
import Regex


receiver : Maybe String -> List AlertGroup -> List AlertGroup
receiver maybeReceiver groups =
    case maybeReceiver of
        Just receiver ->
            by (filterAlertGroup receiver) groups

        Nothing ->
            groups


matchers : Maybe Utils.Types.Matchers -> List AlertGroup -> List AlertGroup
matchers matchers groups =
    case matchers of
        Just ms ->
            by (filterAlertGroupLabels ms) groups

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
            by alertGroupsSilenced groups


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


alertsFromBlock : (Alert -> Bool) -> Block -> Maybe Block
alertsFromBlock fn block =
    let
        alerts =
            List.filter fn block.alerts
    in
        if not <| List.isEmpty alerts then
            Just { block | alerts = alerts }
        else
            Nothing


byLabel : Utils.Types.Matchers -> Block -> Maybe Block
byLabel matchers block =
    alertsFromBlock
        (\a ->
            -- Check that all labels are present within the alert's label set.
            List.all
                (\m ->
                    -- Check for regex or direct match
                    if m.isRegex then
                        -- Check if key is present, then regex match value.
                        let
                            x =
                                List.head <| List.filter (\( key, value ) -> key == m.name) a.labels

                            regex =
                                Regex.regex m.value
                        in
                            -- No regex match
                            case x of
                                Just ( _, value ) ->
                                    Regex.contains regex value

                                Nothing ->
                                    False
                    else
                        List.member ( m.name, m.value ) a.labels
                )
                matchers
        )
        block


filterAlertGroupLabels : Utils.Types.Matchers -> AlertGroup -> Maybe AlertGroup
filterAlertGroupLabels matchers alertGroup =
    let
        blocks =
            List.filterMap (byLabel matchers) alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing


matchersToLabels : Utils.Types.Matchers -> Utils.Types.Labels
matchersToLabels matchers =
    List.map (\m -> ( m.name, m.value )) matchers


alertGroupsSilenced : AlertGroup -> Maybe AlertGroup
alertGroupsSilenced alertGroup =
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
    alertsFromBlock (.silenced >> not) block


by : (a -> Maybe a) -> List a -> List a
by fn groups =
    List.filterMap fn groups
